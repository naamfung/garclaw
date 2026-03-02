package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	net_url "net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// 搜索功能
func Search(keyword string) ([]SearchResult, error) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Headless,
		chromedp.DisableGPU,
	)
	defer cancelAlloc()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctxTimeout, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	defer cancelTimeout()

	searchURL := fmt.Sprintf("https://www.baidu.com/s?ie=UTF-8&wd=%s", keyword)
	results, err := search(ctxTimeout, searchURL)
	if err != nil {
		log.Printf("搜索功能执行失败: %v", err)
		return nil, err
	}
	return results, nil
}

// 访问功能
func Visit(url string) (string, error) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Headless,
		chromedp.DisableGPU,
	)
	defer cancelAlloc()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctxTimeout, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	defer cancelTimeout()

	pageText, err := visitURL(ctxTimeout, url)
	if err != nil {
		log.Printf("访问功能执行失败: %v", err)
		return "", err
	}
	return pageText, nil
}

// 下载小说功能
func DownloadNovel(novelURL string) error {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Headless,
		chromedp.DisableGPU,
	)
	defer cancelAlloc()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// 下载可能耗时较长，不使用超时
	if err := downloadNovel(ctx, novelURL); err != nil {
		log.Printf("下载功能执行失败: %v", err)
		return err
	}
	return nil
}

// 通用下载功能，用于下载网页文件或网页文本
func Download(url string) (string, error) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Headless,
		chromedp.DisableGPU,
	)
	defer cancelAlloc()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctxTimeout, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	defer cancelTimeout()

	// 访问URL并获取页面内容
	var pageContent string
	err := chromedp.Run(ctxTimeout,
		chromedp.Navigate(url),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var content string
			err := chromedp.Evaluate(`document.documentElement.outerHTML`, &content).Do(ctx)
			if err != nil {
				return err
			}
			pageContent = content
			return nil
		}),
	)

	if err != nil {
		log.Printf("下载失败: %v", err)
		return "", err
	}

	// 生成文件名
	fileName := "download_" + time.Now().Format("20060102150405") + ".html"

	// 写入文件
	err = os.WriteFile(fileName, []byte(pageContent), 0644)
	if err != nil {
		log.Printf("保存文件失败: %v", err)
		return "", err
	}

	fmt.Printf("下载完成，保存至: %s\n", fileName)
	return fileName, nil
}

// 搜索结果结构
type SearchResult struct {
	Title string `json:"title"`
	Link  string `json:"link"`
}

// 搜索功能
func search(ctx context.Context, searchURL string) ([]SearchResult, error) {
	// 定义变量来存储搜索结果
	var titles []string
	var links []string

	err := chromedp.Run(ctx,
		chromedp.Navigate(searchURL),
		chromedp.WaitVisible(`#content_left`, chromedp.ByQuery), // 等待搜索结果加载完成
		chromedp.Tasks{
			chromedp.ActionFunc(func(ctx context.Context) error {
				// 获取所有匹配 h3.t a 的节点的 innerText 属性（标题）
				var titleResults []string
				err := chromedp.Evaluate(`Array.from(document.querySelectorAll('h3.t a')).map(a => a.innerText)`, &titleResults).Do(ctx)
				if err != nil {
					return err
				}
				titles = titleResults

				// 获取所有匹配 h3.t a 的节点的 href 属性（链接）
				var linkResults []string
				err = chromedp.Evaluate(`Array.from(document.querySelectorAll('h3.t a')).map(a => a.href)`, &linkResults).Do(ctx)
				if err != nil {
					return err
				}
				links = linkResults

				return nil
			}),
		},
	)
	if err != nil {
		log.Printf("搜索失败: %v", err)
		return nil, err
	}

	// 构建搜索结果
	results := make([]SearchResult, 0, len(titles))
	for i, title := range titles {
		// 打印搜索结果的标题和链接（方便人观察）
		fmt.Printf("Title: %s\nLink: %s\n\n", title, links[i])
		// 添加到结果列表
		results = append(results, SearchResult{
			Title: title,
			Link:  links[i],
		})
	}
	return results, nil
}

// 访问功能
func visitURL(ctx context.Context, url string) (string, error) {
	// Variable to hold the result
	var jsEnabled bool

	// 直接访问URL并获取页面的纯文本内容
	var pageText string
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),     // 等待页面加载完成
		chromedp.Sleep(15*time.Second),                     // 增加等待时间以确保所有内容加载完毕
		chromedp.Evaluate("!!window.document", &jsEnabled), // Check if document object exists (JavaScript is enabled)
		chromedp.ActionFunc(func(ctx context.Context) error {
			if !jsEnabled {
				time.Sleep(15 * time.Second)
			}
			// 获取整个页面的文本内容，排除<script>和<style>标签以及特定的class
			var textContent string
			err := chromedp.Evaluate(`
				function getTextContentWithoutScriptsAndStyles() {
					const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT, null, false);
					let text = '';
					while (walker.nextNode()) {
						const node = walker.currentNode;
						if (!node.parentElement.matches('script, style, .confirm-dialog') &&
							window.getComputedStyle(node.parentElement).display !== 'none' &&
							window.getComputedStyle(node.parentElement).visibility !== 'hidden') {
							text += node.nodeValue.trim() + ' ';
						}
					}
					return text.trim();
				}
				getTextContentWithoutScriptsAndStyles()
			`, &textContent).Do(ctx)
			if err != nil {
				return err
			}

			if jsEnabled {
				textContent = strings.TrimPrefix(textContent, "You need to enable JavaScript to run this app.")
			}

			pageText = textContent
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// 检查是否有JavaScript禁用提示
			var jsDisabledText string
			err := chromedp.Evaluate(`document.querySelector('[role="alert"]')?.innerText || ''`, &jsDisabledText).Do(ctx)
			if err != nil {
				return err
			}
			if strings.Contains(jsDisabledText, "enable JavaScript") {
				log.Printf("Warning: Detected JavaScript disabled message: %s", jsDisabledText)
			}
			return nil
		}),
	)
	if err != nil {
		log.Printf("访问失败: %v", err)
		return "", err
	}

	fmt.Println(pageText)
	return pageText, nil
}

// 下载小说功能
func downloadNovel(ctx context.Context, novelURL string) error {
	log.Printf("开始下载小说: %s\n", novelURL)

	// 先访问小说目录页
	err := chromedp.Run(ctx, chromedp.Navigate(novelURL))
	if err != nil {
		log.Printf("导航到小说页面失败: %v", err)
		return err
	}

	// 等待页面加载完成
	err = chromedp.Run(ctx, chromedp.WaitVisible(`body`, chromedp.ByQuery))
	if err != nil {
		log.Printf("等待页面加载失败: %v", err)
		return err
	}

	// 获取页面标题作为文件名
	var pageTitle string
	err = chromedp.Run(ctx, chromedp.Title(&pageTitle))
	if err != nil {
		log.Printf("获取页面标题失败: %v", err)
		return err
	}

	// 清理标题，使其适合作为文件名
	fileName := cleanFileName(pageTitle) + ".txt"

	// 创建文件
	file, err := os.Create(fileName)
	if err != nil {
		log.Printf("无法创建文件: %v", err)
		return err
	}
	defer file.Close()

	// 获取所有章节链接和标题（用于统计总章节数）
	var chapterList []struct {
		Href string `json:"href"`
		Text string `json:"text"`
	}
	// 使用更宽松的正则表达式匹配章节
	chapterRegex := regexp.MustCompile(`[第卷]([\d一二三四五六七八九十百千]+)[章节回集]`)

	err = chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var allLinks []struct {
				Href string `json:"href"`
				Text string `json:"text"`
			}

			erro := chromedp.Evaluate(
				`Array.from(document.querySelectorAll('a')).map(a => ({href: a.href, text: a.textContent.trim()}))`,
				&allLinks).Do(ctx)
			if erro != nil {
				return erro
			}

			// 收集所有章节链接
			for _, link := range allLinks {
				if chapterRegex.MatchString(link.Text) {
					// 确保链接是绝对路径
					absoluteURL, errn := net_url.Parse(link.Href)
					if errn != nil {
						continue
					}

					// 如果是相对路径，则基于当前URL解析
					baseURL, errn := net_url.Parse(novelURL)
					if errn != nil {
						continue
					}

					absoluteChapterURL := baseURL.ResolveReference(absoluteURL).String()

					chapterList = append(chapterList, struct {
						Href string `json:"href"`
						Text string `json:"text"`
					}{
						Href: absoluteChapterURL,
						Text: link.Text,
					})
				}
			}

			return nil
		}),
	)
	if err != nil {
		log.Printf("获取章节列表失败: %v", err)
	}

	// 获取总章节数
	totalChapterCount := len(chapterList)
	if totalChapterCount == 0 {
		log.Printf("警告: 无法从目录页获取章节列表")
		totalChapterCount = -1 // 无法获取章节总数时设为-1
	} else {
		log.Printf("从目录页找到 %d 个章节", totalChapterCount)
	}

	// 查找第1章的链接
	var firstChapterURL string
	var firstChapterTitle string

	err = chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// 定义结构体变量来接收JavaScript返回的结果
			var chapterResult struct {
				Href string `json:"href"`
				Text string `json:"text"`
			}

			// 查找第1章的链接 - 使用更宽松的正则表达式
			erro := chromedp.Evaluate(`
                function findFirstChapter() {
                    const links = Array.from(document.querySelectorAll('a'));
                    // 尝试多种模式查找第1章
                    const patterns = [
                        /^第1[章节回集]/,
                        /^1[章节回集]/,
                        /^第一章/,
                        /^第一卷/,
                        /^首章/,
                        /^开始阅读/
                    ];

                    // 先尝试找到明确的第1章
                    let link = null;
                    for (const pattern of patterns) {
                        link = links.find(a => {
                            const text = a.textContent.trim();
                            return pattern.test(text) && a.href && !text.includes('目录') && !text.includes('index');
                        });
                        if (link) break;
                    }

                    // 如果找不到明确的第1章，尝试找第一个看起来像章节的链接
                    if (!link) {
                        const chapterPattern = /[第卷]([\d一二三四五六七八九十百千]+)[章节回集]/;
                        const chapterLinks = links.filter(a =>
                            a.href &&
                            chapterPattern.test(a.textContent.trim()) &&
                            !a.textContent.includes('目录') &&
                            !a.textContent.includes('index')
                        );
                        if (chapterLinks.length > 0) {
                            link = chapterLinks[0];
                        }
                    }

                    // 如果还是找不到，尝试找第一个可能是章节列表的容器，然后从里面找第一个链接
                    if (!link) {
                        const chapterContainers = document.querySelectorAll(
                            '.list, .chapter-list, .novel-list, ul, ol'
                        );
                        for (const container of chapterContainers) {
                            const containerLinks = container.querySelectorAll('a');
                            if (containerLinks.length > 5) { // 假设章节列表至少有5个链接
                                link = containerLinks[0];
                                break;
                            }
                        }
                    }

                    if (link) {
                        return {
                            href: link.href,
                            text: link.textContent.trim()
                        };
                    }
                    return null;
                }
                findFirstChapter()
            `, &chapterResult).Do(ctx)

			if erro == nil && chapterResult.Href != "" {
				// 确保链接是绝对路径
				aboluteURL, errn := net_url.Parse(chapterResult.Href)
				if errn != nil {
					return errn
				}

				// 如果是相对路径，则基于当前URL解析
				baseURL, errn := net_url.Parse(novelURL)
				if errn != nil {
					return errn
				}

				firstChapterURL = baseURL.ResolveReference(aboluteURL).String()
				firstChapterTitle = chapterResult.Text
			}

			return erro
		}),
	)

	// 如果找不到第1章的链接，尝试使用第一个符合章节格式的链接
	if firstChapterURL == "" {
		err = chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				var allLinks []struct {
					Href string `json:"href"`
					Text string `json:"text"`
				}

				erro := chromedp.Evaluate(
					`Array.from(document.querySelectorAll('a')).map(a => ({href: a.href, text: a.textContent.trim()}))`,
					&allLinks).Do(ctx)
				if erro != nil {
					return erro
				}

				// 查找第一个符合章节格式的链接
				for _, link := range allLinks {
					if chapterRegex.MatchString(link.Text) {
						// 确保链接是绝对路径
						aboluteURL, errn := net_url.Parse(link.Href)
						if errn != nil {
							continue
						}

						// 如果是相对路径，则基于当前URL解析
						baseURL, errn := net_url.Parse(novelURL)
						if errn != nil {
							continue
						}

						firstChapterURL = baseURL.ResolveReference(aboluteURL).String()
						firstChapterTitle = link.Text
						break
					}
				}

				return nil
			}),
		)
	}

	if firstChapterURL == "" {
		err := fmt.Errorf("无法找到任何章节链接")
		log.Printf("%v", err)
		return err
	}

	fmt.Printf("找到第1章: %s\n", firstChapterTitle)

	// 开始按顺序下载章节内容
	currentChapterURL := firstChapterURL
	currentChapterIndex := 1
	visitedURLs := make(map[string]bool) // 用于记录已访问的URL，避免重复下载
	var currentChapterBaseTitle string   // 用于存储当前章节的基础标题，不包含分页信息
	var currentPageNum int = 1           // 用于跟踪当前章节的页码，在循环外部声明以保持状态

	for {
		// 检查是否已访问过此URL，避免重复下载
		if visitedURLs[currentChapterURL] {
			fmt.Println("检测到重复URL，结束下载")
			break
		}

		// 标记此URL为已访问
		visitedURLs[currentChapterURL] = true

		// 在goto之前声明所有可能被跳过的变量
		var currentChapterTitle string
		var chapterContent string
		var extractedTitle string
		var isSameChapter bool  // 标记当前页面是否是同一章节的分页
		var nextLinkText string // 用于存储下一章链接的文本内容

		// 为当前章节创建独立的超时上下文（5分钟）
		chapterCtx, chapterCancel := context.WithTimeout(ctx, 300*time.Second)
		defer chapterCancel()

		// 访问当前章节页面 - 增加重试逻辑
		var navigationSuccess bool
		const maxRetries = 3
		for retry := 0; retry < maxRetries; retry++ {
			err = chromedp.Run(chapterCtx, chromedp.Navigate(currentChapterURL))
			if err != nil {
				log.Printf("第%d次访问章节失败: %v", retry+1, err)
				if retry < maxRetries-1 {
					// 指数退避策略：第一次等待10秒，第二次20秒，第三次40秒
					waitTime := time.Duration(10*(1<<retry)) * time.Second
					log.Printf("等待%v后重试...", waitTime)
					time.Sleep(waitTime)
					continue
				}
				// 最后一次重试也失败，才跳转到下一章
				log.Printf("所有重试都失败，尝试跳过本章节...")
				goto NextChapter
			}

			// 等待页面加载完成
			err = chromedp.Run(chapterCtx, chromedp.WaitVisible(`body`, chromedp.ByQuery))
			if err != nil {
				log.Printf("等待章节加载失败: %v", err)
				if retry < maxRetries-1 {
					time.Sleep(10 * time.Second)
					continue
				}
				goto NextChapter
			}

			// 导航和加载都成功
			navigationSuccess = true
			break
		}

		// 如果所有重试都失败，直接跳转到下一章
		if !navigationSuccess {
			log.Printf("无法访问章节，尝试跳过本章节: %s", currentChapterURL)
			// 在文件中记录错误信息
			errorMsg := fmt.Sprintf("【错误】无法访问章节: %s (URL: %s)\n\n", currentChapterTitle, currentChapterURL)
			file.WriteString(errorMsg)

			// 尝试使用目录页中的下一章链接
			if len(chapterList) > 0 && currentChapterIndex < len(chapterList) {
				nextChapterURL := chapterList[currentChapterIndex].Href
				fmt.Printf("使用目录页中的下一章链接: %s\n", nextChapterURL)

				// 更新当前章节信息
				currentChapterURL = nextChapterURL
				currentChapterIndex++

				// 添加随机延迟
				randomDelay := time.Duration(5+rand.Intn(56)) * time.Second
				fmt.Printf("等待 %v 后尝试下一章...\n", randomDelay)
				time.Sleep(randomDelay)
				continue
			} else {
				fmt.Println("无法获取下一章链接，下载完成")
				break
			}
		}

		// 获取当前章节标题 - 改进版本，尝试从页面内容中提取更准确的标题
		err = chromedp.Run(chapterCtx, chromedp.Title(&currentChapterTitle))
		if err != nil {
			log.Printf("获取章节标题失败: %v", err)
			currentChapterTitle = fmt.Sprintf("第%d章", currentChapterIndex)
		} else {
			// 尝试从页面内容中提取更准确的章节标题
			err = chromedp.Run(chapterCtx, chromedp.Evaluate(`
                function extractChapterTitle() {
                    // 首先检查h1-h3标题标签中是否有章节标题
                    let chapterTitle = null;
                    const titleElements = Array.from(document.querySelectorAll('h1, h2, h3'));
                    const chapterPattern = /第\d+[章节回]/;

                    for (const element of titleElements) {
                        if (chapterPattern.test(element.textContent.trim())) {
                            chapterTitle = element.textContent.trim();
                            break;
                        }
                    }

                    // 如果没找到，再检查body中的文本节点
                    if (!chapterTitle) {
                        const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT, null, false);
                        let text = '';
                        while (walker.nextNode()) {
                            const node = walker.currentNode;
                            text += node.nodeValue;
                        }

                        const match = text.match(/第\d+[章节回][^\n]+/);
                        if (match && match[0]) {
                            chapterTitle = match[0].trim();
                        }
                    }

                    return chapterTitle;
                }
                extractChapterTitle()
            `, &extractedTitle))
			// 尝试从页面内容中提取更准确的章节标题
			if err == nil && extractedTitle != "" {
				currentChapterTitle = extractedTitle
			}
		}

		// 检查是否是同一章节的分页
		if currentChapterBaseTitle == "" {
			// 首次设置章节基础标题，去除可能的分页信息
			currentChapterBaseTitle = extractBaseChapterTitle(currentChapterTitle)
			isSameChapter = false
			currentPageNum = 1 // 重置页码计数
		} else {
			// 比较当前页面标题与基础标题
			currentBaseTitle := extractBaseChapterTitle(currentChapterTitle)
			isSameChapter = (currentBaseTitle == currentChapterBaseTitle)
		}

		// 格式化当前章节标题和输出
		if isSameChapter {
			formattedTitle := fmt.Sprintf("%s_第%d页", currentChapterBaseTitle, currentPageNum)
			fmt.Printf("正在下载第 %d 章: %s\n", currentChapterIndex, formattedTitle)
		} else {
			fmt.Printf("正在下载第 %d 章: %s\n", currentChapterIndex, currentChapterTitle)
			// 更新基础标题
			currentChapterBaseTitle = extractBaseChapterTitle(currentChapterTitle)
			// 重置页码计数
			currentPageNum = 1
		}

		// 获取章节内容 - 改进版本，尝试找到包含大量连续文本的容器
		err = chromedp.Run(chapterCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				var content string
				erro := chromedp.Evaluate(`
                    function getChapterContent() {
                        // 尝试找到包含大量连续文本且包含多个段落的容器
                        const elements = document.querySelectorAll('div, article, section, span, pre, li, blockquote, main');
                        let bestCandidate = null;
                        let maxTextLength = 0;

                        for (const element of elements) {
                            // 跳过隐藏元素和不需要的元素
                            if (window.getComputedStyle(element).display === 'none' ||
                                window.getComputedStyle(element).visibility === 'hidden' ||
                                window.getComputedStyle(element).opacity === '0' ||
                                element.matches('script, style, .confirm-dialog, nav, footer, header, aside')) {
                                continue;
                            }

                            const text = element.textContent.trim();
                            const textLength = text.length;

                            //跳过：条件1：同一行内同时包含「作者：」「分类：」「更新：」「字数：」的容器
                            if (text.includes('作者：') && text.includes('分类：') && text.includes('更新：') && text.includes('字数：')) {
                                continue;
                            }

                            //跳过：条件2：同一行内同时包含｛「上一章」或「上一页」｝「目录」｛「下一章」或「下一页」｝的容器
                            if ((text.includes('上一章')||text.includes('上一页')) && text.includes('目录') && (text.includes('下一章')||text.includes('下一页'))) {
                                continue;
                            }

                            //跳过：条件3：包含「投推荐票」或「加入书签」的容器
                            if (text.includes('投推荐票')||text.includes('加入书签')) {
                                continue;
                            }

                            // 如果文本长度足够长，并且比当前最佳候选更长
                            if (textLength > 300 && textLength > maxTextLength) {
                                // 检查文本质量：连续文本比例和段落数量
                                const lineBreakCount = (text.match(/\n/g) || []).length;
                                const paragraphCount = lineBreakCount + 1; // 假设每行一个段落

                                // 如果文本质量较好，更新最佳候选
                                if (textLength / paragraphCount > 50) {
                                    bestCandidate = element;
                                    maxTextLength = textLength;
                                }
                            }
                        }

                        // 定义文末可能存在且须要被去除的无关内容
                        const delTextsOfEnd = ['上一章', '上一页', '目录', '目 录', '下一章', '下一页', '点击下一页继续阅读', '小说网更新速度全网最快。'];

                        // 如果找到了合适的容器，提取其文本内容并保留段落格式
                        if (bestCandidate) {
                            // 使用 TreeWalker 提取文本内容并保留段落格式
                            const walker = document.createTreeWalker(bestCandidate, NodeFilter.SHOW_TEXT, null, false);
                            let text = '';
                            while (walker.nextNode()) {
                                const node = walker.currentNode;
                                if (!node.parentElement.matches('script, style, .confirm-dialog') &&
                                    window.getComputedStyle(node.parentElement).display !== 'none' &&
                                    window.getComputedStyle(node.parentElement).visibility !== 'hidden' &&
                                    window.getComputedStyle(node.parentElement).opacity !== '0') {
                                    text += node.nodeValue.trim() + '\n';
                                }
                            }
                            // 清理多余的空白字符，但保留段落格式
                            text = text.trim().replace(/[^\S\n]+/g, ' ');

                            // 从正文末尾倒序检查并去除导航部分
                            const lines = text.split('\n');
                            let cleanedText = '';

                            // 只检查最后的10行内容
                            const startLine = Math.max(0, lines.length - 10);
                            for (let i = lines.length - 1; i >= startLine; i--) {
                                if (!delTextsOfEnd.some(navigationText => lines[i].trim().includes(navigationText))) {
                                    cleanedText = lines[i] + (cleanedText ? '\n' + cleanedText : '');
                                }
                            }

                            // 合并剩余的文本
                            cleanedText = lines.slice(0, startLine).join('\n') + (cleanedText ? '\n' + cleanedText : '');

                            return cleanedText.trim();
                        }

                        // 如果未有找到合适的容器，回退到原来的方法
                        const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT, null, false);
                        let text = '';
                        while (walker.nextNode()) {
                            const node = walker.currentNode;
                            if (!node.parentElement.matches('script, style, .confirm-dialog, nav, footer, header, aside') &&
                                window.getComputedStyle(node.parentElement).display !== 'none' &&
                                window.getComputedStyle(node.parentElement).visibility !== 'hidden' &&
                                window.getComputedStyle(node.parentElement).opacity !== '0') {
                                text += node.nodeValue.trim() + '\n';
                            }
                        }
                        // 清理多余的空白字符，但保留段落格式
                        text = text.trim().replace(/[^\S\n]+/g, ' ');

                        // 从正文末尾倒序检查并去除导航部分
                        const lines = text.split('\n');
                        let cleanedText = '';

                        // 只检查最后的10行内容
                        const startLine = Math.max(0, lines.length - 10);
                        for (let i = lines.length - 1; i >= startLine; i--) {
                            if (!delTextsOfEnd.some(navigationText => lines[i].trim().includes(navigationText))) {
                                cleanedText = lines[i] + (cleanedText ? '\n' + cleanedText : '');
                            }
                        }

                        // 合并剩余的文本
                        cleanedText = lines.slice(0, startLine).join('\n') + (cleanedText ? '\n' + cleanedText : '');

                        return cleanedText.trim();
                    }
                    getChapterContent()
                `, &content).Do(ctx)
				if erro != nil {
					return erro
				}
				chapterContent = content
				return nil
			}),
		)

		if err != nil {
			log.Printf("获取章节内容失败: %v, 跳过...", err)
			// 在文件中记录错误信息
			errorMsg := fmt.Sprintf("【错误】获取章节内容失败: %s (URL: %s)\n\n", currentChapterTitle, currentChapterURL)
			file.WriteString(errorMsg)
			// 尝试继续查找下一章
			goto NextChapter
		}

		// 写入文件
		if !isSameChapter {
			// 新章节，写入标题和内容
			_, err = file.WriteString(fmt.Sprintf("%s\n\n%s\n\n", currentChapterTitle, chapterContent))
		} else {
			// 同一章节的分页，只写入内容，不写入标题
			_, err = file.WriteString(fmt.Sprintf("%s\n\n", chapterContent))
		}
		if err != nil {
			log.Printf("写入章节内容失败: %v", err)
		}

	NextChapter:
		// 查找下一章的链接
		var nextChapterURL string

		err = chromedp.Run(chapterCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				var nextLink string

				// 先模拟滚动条滚动到底部，使之更似人类行为
				scrollAction := chromedp.Evaluate(`
                    function humanScrollToBottom() {
                        // 使用带随机性质的非线性公式进行滚动
                        const duration = 2000 + Math.random() * 3000; // 滚动持续时间在2-5秒之间
                        const startTime = Date.now();
                        const startScroll = window.scrollY;
                        const endScroll = document.body.scrollHeight - window.innerHeight;
                        const distance = endScroll - startScroll;

                        // 非线性滚动函数
                        function easeInOutCubic(t) {
                            return t < 0.5 ? 4 * t * t * t : (t - 1) * (2 * t - 2) * (2 * t - 2) + 1;
                        }

                        // 添加一些随机波动的函数
                        function addRandomness(progress) {
                            const randomFactor = 1 + (Math.random() - 0.5) * 0.2; // -10%到+10%的随机波动
                            return progress * randomFactor;
                        }

                        function scrollStep() {
                            const elapsed = Date.now() - startTime;
                            let progress = Math.min(elapsed / duration, 1);
                            progress = easeInOutCubic(progress);
                            progress = addRandomness(progress);
                            window.scrollTo(0, startScroll + distance * progress);

                            if (progress < 1) {
                                requestAnimationFrame(scrollStep);
                            }
                        }

                        // 开始滚动
                        scrollStep();

                        // 返回一个Promise来让Go代码等待滚动完成
                        return new Promise(resolve => {
                            setTimeout(resolve, duration + 500); // 额外等待500ms以确保滚动完成
                        });
                    }
                    humanScrollToBottom()
                `, nil)

				// 执行滚动操作并捕获可能的错误
				erro := chromedp.Run(ctx, scrollAction)
				if erro != nil {
					return erro
				}

				// 尝试多种方式查找下一章链接
				erro2 := chromedp.Run(ctx, chromedp.Evaluate(`
                    function findNextChapter() {
                        // 方法1: 查找包含'下一章'或'下一页'文字的链接
                        const nextChapterKeywords = ['下一章', '下一页', '下节', '下一话', '下一回'];
                        let link = null;

                        for (const keyword of nextChapterKeywords) {
                            link = Array.from(document.querySelectorAll('a')).find(a =>
                                a.textContent.includes(keyword) && a.href);
                            if (link) break;
                        }

                        // 方法2: 查找id或class包含'next'的链接
                        if (!link) {
                            link = document.querySelector('a[id*="next" i], a[class*="next" i]');
                        }

                        // 方法3: 查找第X+1章的链接
                        if (!link) {
                            const currentChapterText = document.title || '';
                            const chapterNumMatch = currentChapterText.match(/第(\d+)[章节回]/);
                            let nextChapterNum = 0;
                            if (chapterNumMatch && chapterNumMatch[1]) {
                                nextChapterNum = parseInt(chapterNumMatch[1]) + 1;
                            } else {
                                // 如果无法从标题中提取章节号，使用默认值
                                nextChapterNum = 1000; // 使用一个较大的数字，希望能找到一些链接
                            }
                            const nextChapterPattern = new RegExp('第' + nextChapterNum + '[章节回]');
                            link = Array.from(document.querySelectorAll('a')).find(a =>
                                nextChapterPattern.test(a.textContent));
                        }

                        // 方法4: 查找rel="next"的链接
                        if (!link) {
                            link = document.querySelector('a[rel="next"]');
                        }

                        // 检查找到的链接是否是推荐链接或非章节链接
                        if (link) {
                            const href = link.href;
                            const text = link.textContent.trim();

                            // 排除推荐链接和非章节链接
                            const excludePatterns = [
                                /recommend/i,
                                /related/i,
                                /tuijian/i,
                                /xiaoshuo/i,
                                /book/i,
                                /index/i,
                                /目录/i,
                                /首页/i,
                                /home/i,
                                /list/i
                            ];

                            for (const pattern of excludePatterns) {
                                if (pattern.test(href) || pattern.test(text)) {
                                    return null;
                                }
                            }

                            return href;
                        }

                        return null;
                    }
                    findNextChapter()
                `, &nextLink))

				if erro2 == nil && nextLink != "" {
					// 确保链接是绝对路径
					aboluteURL, errn := net_url.Parse(nextLink)
					if errn != nil {
						return errn
					}

					// 如果是相对路径，则基于当前URL解析
					baseURL, errn := net_url.Parse(currentChapterURL)
					if errn != nil {
						return errn
					}

					nextChapterURL = baseURL.ResolveReference(aboluteURL).String()
				}

				return erro2
			}),
		)

		// 如果找不到下一章链接，尝试使用目录页的章节列表
		if nextChapterURL == "" && len(chapterList) > 0 && currentChapterIndex < len(chapterList) {
			nextChapterURL = chapterList[currentChapterIndex].Href
			fmt.Printf("使用目录页的链接作为下一章: %s\n", nextChapterURL)
		}

		// 如果还是找不到下一章链接，结束下载
		if nextChapterURL == "" {
			fmt.Println("未找到下一章链接，下载完成")
			break
		}

		// 检查是否已达到总章节数
		if totalChapterCount > 0 && currentChapterIndex >= totalChapterCount {
			fmt.Println("已下载所有章节，下载完成")
			break
		}

		// 为了避免请求过快，添加5到60秒的随机延迟
		randomDelay := time.Duration(5+rand.Intn(56)) * time.Second
		// 先获取下一章链接文本并判断是否为同一章节
		nextLinkText = ""
		// 使用chapterCtx替代ctx，避免上下文超时
		err = chromedp.Run(chapterCtx, chromedp.Evaluate(`
            function getNextLinkText() {
                // 查找导航按钮区域中的页码信息
                const paginationElements = document.querySelectorAll(
                    '.pagination, .pager, [id*="page"], [class*="page"], [role="navigation"], a[href*="page="], a[href*="p="]'
                );

                // 查找包含页码信息的导航元素
                for (let i = 0; i < paginationElements.length; i++) {
                    const text = paginationElements[i].textContent;
                    if (text.includes('第') && (text.includes('页') || text.includes('/'))) {
                        return text;
                    }
                }

                // 如果没有找到导航区域的页码，再按原来的方式查找下一章链接文本
                const nextChapterKeywords = ['下一章', '下一页', '下节', '下一话', '下一回'];
                let link = null;
                for (const keyword of nextChapterKeywords) {
                    link = Array.from(document.querySelectorAll('a')).find(a =>
                        a.textContent.includes(keyword) && a.href);
                    if (link) return link.textContent;
                }
                return '';
            }
            getNextLinkText()
        `, &nextLinkText))

		// 如果导航文本中包含页码信息，提取并使用它
		pageMatch := regexp.MustCompile(`第(\d+)[页\/]`).FindStringSubmatch(nextLinkText)
		if len(pageMatch) > 1 {
			pageNumFromNav, _ := strconv.Atoi(pageMatch[1])
			if pageNumFromNav > currentPageNum {
				currentPageNum = pageNumFromNav
			}
		}

		// 判断下一个链接是下一页还是下一章
		if strings.Contains(nextLinkText, "下一页") || strings.Contains(nextLinkText, "页") {
			// 如果是下一页，强制设置为同一章节
			isSameChapter = true
		} else {
			// 不是下一页，重置页码计数器
			currentPageNum = 1
			isSameChapter = false
		}

		// 更新当前章节信息
		currentChapterURL = nextChapterURL
		// 只有当不是同一章节的分页时才增加章节索引
		if !isSameChapter {
			currentChapterIndex++
		} else {
			currentPageNum++
		}

		// 显示相应的等待提示
		if isSameChapter {
			fmt.Printf("等待 %v 后下载下一页...\n", randomDelay)
		} else {
			fmt.Printf("等待 %v 后下载下一章...\n", randomDelay)
		}
		time.Sleep(randomDelay)
	}

	fmt.Printf("小说下载完成，保存至: %s\n", fileName)
	return nil
}

// 提取章节的基础标题，去除可能的分页信息
func extractBaseChapterTitle(title string) string {
	// 使用正则表达式匹配并移除常见的分页模式
	// 匹配如：(1/5), 第1页, 分页1等模式
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\(\d+/\d+\)`),
		regexp.MustCompile(`第\d+页`),
		regexp.MustCompile(`分页\d+`),
		regexp.MustCompile(`\[\d+/\d+\]`),
		regexp.MustCompile(`\d+/\d+`),
	}

	baseTitle := title
	for _, pattern := range patterns {
		baseTitle = pattern.ReplaceAllString(baseTitle, "")
		// 移除替换后可能产生的多余空格
		baseTitle = strings.TrimSpace(baseTitle)
	}

	// 如果没有找到分页信息，返回原始标题
	return baseTitle
}

// 清理文件名，移除不合法字符
func cleanFileName(name string) string {
	// 替换不合法的文件名字符
	invalidChars := regexp.MustCompile(`[<>:"/\|?*]`)
	cleaned := invalidChars.ReplaceAllString(name, "_")
	// 移除多余的下划线
	cleaned = regexp.MustCompile(`_+`).ReplaceAllString(cleaned, "_")
	// 移除首尾的下划线
	cleaned = strings.Trim(cleaned, "_")
	return cleaned
}

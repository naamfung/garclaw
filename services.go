package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	net_url "net/url"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

func init() {
	playwright.Install()
}

// 搜索结果结构
type SearchResult struct {
	Title string `json:"title"`
	Link  string `json:"link"`
}

// 搜索功能
func Search(keyword string) ([]SearchResult, error) {
	pw, err := playwright.Run()
	if err != nil {
		log.Printf("启动 Playwright 失败: %v", err)
		return nil, err
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		if runtime.GOOS == "linux" {
			browser, err = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
				Headless:       playwright.Bool(true),                          // 是否无头模式
				ExecutablePath: playwright.String("/usr/bin/chromium-browser"), // 根据实际路径调整
				Args: []string{
					"--no-sandbox",
					"--disable-setuid-sandbox",
				},
			})
			if err != nil {
				log.Printf("启动浏览器失败: %v", err)
				return nil, err
			}
		}

		log.Printf("启动浏览器失败: %v", err)
		return nil, err
	}
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		log.Printf("创建页面失败: %v", err)
		return nil, err
	}
	defer page.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	page.SetDefaultTimeout(float64(60 * time.Second / time.Millisecond))

	searchURL := fmt.Sprintf("https://www.baidu.com/s?ie=UTF-8&wd=%s", keyword)
	return search(ctx, page, searchURL)
}

// 访问功能
func Visit(url string) (string, error) {
	pw, err := playwright.Run()
	if err != nil {
		log.Printf("启动 Playwright 失败: %v", err)
		return "", err
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		if runtime.GOOS == "linux" {
			browser, err = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
				Headless:       playwright.Bool(true),                          // 是否无头模式
				ExecutablePath: playwright.String("/usr/bin/chromium-browser"), // 根据实际路径调整
				Args: []string{
					"--no-sandbox",
					"--disable-setuid-sandbox",
				},
			})
			if err != nil {
				log.Printf("启动浏览器失败: %v", err)
				return "", err
			}
		}

		log.Printf("启动浏览器失败: %v", err)
		return "", err
	}
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		log.Printf("创建页面失败: %v", err)
		return "", err
	}
	defer page.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	page.SetDefaultTimeout(float64(60 * time.Second / time.Millisecond))

	return visitURL(ctx, page, url)
}

// 下载小说功能
func DownloadNovel(novelURL string) error {
	pw, err := playwright.Run()
	if err != nil {
		log.Printf("启动 Playwright 失败: %v", err)
		return err
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		if runtime.GOOS == "linux" {
			browser, err = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
				Headless:       playwright.Bool(true),                          // 是否无头模式
				ExecutablePath: playwright.String("/usr/bin/chromium-browser"), // 根据实际路径调整
				Args: []string{
					"--no-sandbox",
					"--disable-setuid-sandbox",
				},
			})
			if err != nil {
				log.Printf("启动浏览器失败: %v", err)
				return err
			}
		}

		log.Printf("启动浏览器失败: %v", err)
		return err
	}
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		log.Printf("创建页面失败: %v", err)
		return err
	}
	defer page.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	page.SetDefaultTimeout(float64(30 * time.Minute / time.Millisecond))

	return downloadNovel(ctx, page, novelURL)
}

// 通用下载功能
func Download(url string) (string, error) {
	pw, err := playwright.Run()
	if err != nil {
		log.Printf("启动 Playwright 失败: %v", err)
		return "", err
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		if runtime.GOOS == "linux" {
			browser, err = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
				Headless:       playwright.Bool(true),                          // 是否无头模式
				ExecutablePath: playwright.String("/usr/bin/chromium-browser"), // 根据实际路径调整
				Args: []string{
					"--no-sandbox",
					"--disable-setuid-sandbox",
				},
			})
			if err != nil {
				log.Printf("启动浏览器失败: %v", err)
				return "", err
			}
		}

		log.Printf("启动浏览器失败: %v", err)
		return "", err
	}
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		log.Printf("创建页面失败: %v", err)
		return "", err
	}
	defer page.Close()

	// ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	// defer cancel()
	page.SetDefaultTimeout(float64(60 * time.Second / time.Millisecond))

	if _, err = page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		log.Printf("导航失败: %v", err)
		return "", err
	}

	if _, err = page.WaitForSelector("body", playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateAttached,
	}); err != nil {
		log.Printf("等待 body 失败: %v", err)
		return "", err
	}

	time.Sleep(2 * time.Second)

	pageContent, err := page.Content()
	if err != nil {
		log.Printf("获取页面内容失败: %v", err)
		return "", err
	}

	fileName := "download_" + time.Now().Format("20060102150405") + ".html"
	err = os.WriteFile(fileName, []byte(pageContent), 0644)
	if err != nil {
		log.Printf("保存文件失败: %v", err)
		return "", err
	}

	fmt.Printf("下载完成，保存至: %s\n", fileName)
	return fileName, nil
}

// 内部搜索实现
func search(ctx context.Context, page playwright.Page, searchURL string) ([]SearchResult, error) {
	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if _, err := page.Goto(searchURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		log.Printf("导航到搜索页失败: %v", err)
		return nil, err
	}

	if _, err := page.WaitForSelector("#content_left", playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateAttached,
	}); err != nil {
		log.Printf("等待搜索结果容器失败: %v", err)
		return nil, err
	}

	var titles []string
	var links []string

	titlesResult, err := page.Evaluate(`Array.from(document.querySelectorAll('h3.t a')).map(a => a.innerText)`)
	if err != nil {
		log.Printf("提取标题失败: %v", err)
		return nil, err
	}
	if titlesResult != nil {
		for _, v := range titlesResult.([]interface{}) {
			titles = append(titles, v.(string))
		}
	}

	linksResult, err := page.Evaluate(`Array.from(document.querySelectorAll('h3.t a')).map(a => a.href)`)
	if err != nil {
		log.Printf("提取链接失败: %v", err)
		return nil, err
	}
	if linksResult != nil {
		for _, v := range linksResult.([]interface{}) {
			links = append(links, v.(string))
		}
	}

	results := make([]SearchResult, 0, len(titles))
	for i, title := range titles {
		fmt.Printf("Title: %s\nLink: %s\n\n", title, links[i])
		results = append(results, SearchResult{
			Title: title,
			Link:  links[i],
		})
	}
	return results, nil
}

// 内部访问实现
func visitURL(ctx context.Context, page playwright.Page, url string) (string, error) {
	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		log.Printf("导航到页面失败: %v", err)
		return "", err
	}

	if _, err := page.WaitForSelector("body", playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateAttached,
	}); err != nil {
		log.Printf("等待 body 失败: %v", err)
		return "", err
	}

	time.Sleep(15 * time.Second)

	jsEnabled := true
	textContent, err := page.Evaluate(`
        (() => {
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
        })()
    `)
	if err != nil {
		log.Printf("提取文本内容失败: %v", err)
		return "", err
	}

	pageText := textContent.(string)

	if jsEnabled {
		pageText = strings.TrimPrefix(pageText, "You need to enable JavaScript to run this app.")
	}

	jsDisabledText, err := page.Evaluate(`document.querySelector('[role="alert"]')?.innerText || ''`)
	if err == nil && jsDisabledText != nil && jsDisabledText.(string) != "" {
		if strings.Contains(jsDisabledText.(string), "enable JavaScript") {
			log.Printf("Warning: Detected JavaScript disabled message: %s", jsDisabledText)
		}
	}

	if len(pageText) > 512 {
		fmt.Println("Page content (truncated): " + pageText[:512] + "...")
	} else {
		fmt.Println(pageText)
	}
	return pageText, nil
}

// 内部下载小说实现
func downloadNovel(ctx context.Context, page playwright.Page, novelURL string) error {
	log.Printf("开始下载小说: %s\n", novelURL)

	if _, err := page.Goto(novelURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		log.Printf("导航到小说页面失败: %v", err)
		return err
	}

	if _, err := page.WaitForSelector("body", playwright.PageWaitForSelectorOptions{
		State: playwright.WaitForSelectorStateAttached,
	}); err != nil {
		log.Printf("等待页面加载失败: %v", err)
		return err
	}

	pageTitle, err := page.Title()
	if err != nil {
		log.Printf("获取页面标题失败: %v", err)
		return err
	}
	fileName := cleanFileName(pageTitle) + ".txt"

	file, err := os.Create(fileName)
	if err != nil {
		log.Printf("无法创建文件: %v", err)
		return err
	}
	defer file.Close()

	chapterRegex := regexp.MustCompile(`[第卷]([\d一二三四五六七八九十百千]+)[章节回集]`)

	var chapterList []struct {
		Href string
		Text string
	}

	allLinks, err := page.Evaluate(`Array.from(document.querySelectorAll('a')).map(a => ({href: a.href, text: a.textContent.trim()}))`)
	if err != nil {
		log.Printf("获取所有链接失败: %v", err)
	} else {
		for _, linkIf := range allLinks.([]interface{}) {
			linkMap := linkIf.(map[string]interface{})
			href := linkMap["href"].(string)
			text := linkMap["text"].(string)

			if chapterRegex.MatchString(text) {
				absoluteURL, err := net_url.Parse(href)
				if err != nil {
					continue
				}
				baseURL, err := net_url.Parse(novelURL)
				if err != nil {
					continue
				}
				absoluteChapterURL := baseURL.ResolveReference(absoluteURL).String()
				chapterList = append(chapterList, struct {
					Href string
					Text string
				}{
					Href: absoluteChapterURL,
					Text: text,
				})
			}
		}
	}

	totalChapterCount := len(chapterList)
	if totalChapterCount == 0 {
		log.Printf("警告: 无法从目录页获取章节列表")
		totalChapterCount = -1
	} else {
		log.Printf("从目录页找到 %d 个章节", totalChapterCount)
	}

	var firstChapterURL string
	var firstChapterTitle string

	chapterResult, err := page.Evaluate(`
		function findFirstChapter() {
			const links = Array.from(document.querySelectorAll('a'));
			const patterns = [
				/^第1[章节回集]/,
				/^1[章节回集]/,
				/^第一章/,
				/^第一卷/,
				/^首章/,
				/^开始阅读/
			];

			let link = null;
			for (const pattern of patterns) {
				link = links.find(a => {
					const text = a.textContent.trim();
					return pattern.test(text) && a.href && !text.includes('目录') && !text.includes('index');
				});
				if (link) break;
			}

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

			if (!link) {
				const chapterContainers = document.querySelectorAll(
					'.list, .chapter-list, .novel-list, ul, ol'
				);
				for (const container of chapterContainers) {
					const containerLinks = container.querySelectorAll('a');
					if (containerLinks.length > 5) {
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
	`)

	if err == nil && chapterResult != nil {
		resultMap := chapterResult.(map[string]interface{})
		if href, ok := resultMap["href"].(string); ok && href != "" {
			firstChapterURL = href
			firstChapterTitle = resultMap["text"].(string)
			absURL, err := net_url.Parse(firstChapterURL)
			if err == nil {
				baseURL, _ := net_url.Parse(novelURL)
				firstChapterURL = baseURL.ResolveReference(absURL).String()
			}
		}
	}

	if firstChapterURL == "" && len(chapterList) > 0 {
		firstChapterURL = chapterList[0].Href
		firstChapterTitle = chapterList[0].Text
	}

	if firstChapterURL == "" {
		err := fmt.Errorf("无法找到任何章节链接")
		log.Printf("%v", err)
		return err
	}

	fmt.Printf("找到第1章: %s\n", firstChapterTitle)

	currentChapterURL := firstChapterURL
	currentChapterIndex := 1
	visitedURLs := make(map[string]bool)
	var currentChapterBaseTitle string
	var currentPageNum = 1

	// 声明将在循环内使用的变量，以避免 goto 跳过声明
	var currentChapterTitle string
	var chapterContent interface{}
	var nextLinkText interface{}

	for {
		if visitedURLs[currentChapterURL] {
			fmt.Println("检测到重复URL，结束下载")
			break
		}
		visitedURLs[currentChapterURL] = true

		var navigationSuccess bool
		const maxRetries = 3
		for retry := 0; retry < maxRetries; retry++ {
			_, err := page.Goto(currentChapterURL, playwright.PageGotoOptions{
				WaitUntil: playwright.WaitUntilStateNetworkidle,
			})
			if err != nil {
				log.Printf("第%d次访问章节失败: %v", retry+1, err)
				if retry < maxRetries-1 {
					waitTime := time.Duration(10*(1<<retry)) * time.Second
					log.Printf("等待%v后重试...", waitTime)
					time.Sleep(waitTime)
					continue
				}
				log.Printf("所有重试都失败，尝试跳过本章节...")
				goto NextChapter
			}

			_, err = page.WaitForSelector("body", playwright.PageWaitForSelectorOptions{
				State: playwright.WaitForSelectorStateAttached,
			})
			if err != nil {
				log.Printf("等待章节加载失败: %v", err)
				if retry < maxRetries-1 {
					time.Sleep(10 * time.Second)
					continue
				}
				goto NextChapter
			}

			navigationSuccess = true
			break
		}

		if !navigationSuccess {
			log.Printf("无法访问章节，尝试跳过本章节: %s", currentChapterURL)
			errorMsg := fmt.Sprintf("【错误】无法访问章节: (URL: %s)\n\n", currentChapterURL)
			file.WriteString(errorMsg)

			if len(chapterList) > 0 && currentChapterIndex < len(chapterList) {
				nextChapterURL := chapterList[currentChapterIndex].Href
				fmt.Printf("使用目录页中的下一章链接: %s\n", nextChapterURL)
				currentChapterURL = nextChapterURL
				currentChapterIndex++
				randomDelay := time.Duration(5+rand.Intn(56)) * time.Second
				fmt.Printf("等待 %v 后尝试下一章...\n", randomDelay)
				time.Sleep(randomDelay)
				continue
			} else {
				fmt.Println("无法获取下一章链接，下载完成")
				break
			}
		}

		currentChapterTitle, err = page.Title()
		if err != nil {
			log.Printf("获取章节标题失败: %v", err)
			currentChapterTitle = fmt.Sprintf("第%d章", currentChapterIndex)
		} else {
			extractedTitle, err := page.Evaluate(`
				function extractChapterTitle() {
					let chapterTitle = null;
					const titleElements = Array.from(document.querySelectorAll('h1, h2, h3'));
					const chapterPattern = /第\d+[章节回]/;

					for (const element of titleElements) {
						if (chapterPattern.test(element.textContent.trim())) {
							chapterTitle = element.textContent.trim();
							break;
						}
					}

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
			`)
			if err == nil && extractedTitle != nil && extractedTitle.(string) != "" {
				currentChapterTitle = extractedTitle.(string)
			}
		}

		if currentChapterBaseTitle == "" {
			currentChapterBaseTitle = extractBaseChapterTitle(currentChapterTitle)
			currentPageNum = 1
		} else {
			currentBaseTitle := extractBaseChapterTitle(currentChapterTitle)
			if currentBaseTitle == currentChapterBaseTitle {
				formattedTitle := fmt.Sprintf("%s_第%d页", currentChapterBaseTitle, currentPageNum)
				fmt.Printf("正在下载第 %d 章: %s\n", currentChapterIndex, formattedTitle)
			} else {
				fmt.Printf("正在下载第 %d 章: %s\n", currentChapterIndex, currentChapterTitle)
				currentChapterBaseTitle = currentBaseTitle
				currentPageNum = 1
			}
		}

		chapterContent, err = page.Evaluate(`
			function getChapterContent() {
				const elements = document.querySelectorAll('div, article, section, span, pre, li, blockquote, main');
				let bestCandidate = null;
				let maxTextLength = 0;

				for (const element of elements) {
					if (window.getComputedStyle(element).display === 'none' ||
						window.getComputedStyle(element).visibility === 'hidden' ||
						window.getComputedStyle(element).opacity === '0' ||
						element.matches('script, style, .confirm-dialog, nav, footer, header, aside')) {
						continue;
					}

					const text = element.textContent.trim();
					const textLength = text.length;

					if (text.includes('作者：') && text.includes('分类：') && text.includes('更新：') && text.includes('字数：')) continue;
					if ((text.includes('上一章')||text.includes('上一页')) && text.includes('目录') && (text.includes('下一章')||text.includes('下一页'))) continue;
					if (text.includes('投推荐票')||text.includes('加入书签')) continue;

					if (textLength > 300 && textLength > maxTextLength) {
						const lineBreakCount = (text.match(/\n/g) || []).length;
						const paragraphCount = lineBreakCount + 1;
						if (textLength / paragraphCount > 50) {
							bestCandidate = element;
							maxTextLength = textLength;
						}
					}
				}

				const delTextsOfEnd = ['上一章', '上一页', '目录', '目 录', '下一章', '下一页', '点击下一页继续阅读', '小说网更新速度全网最快。'];

				if (bestCandidate) {
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
					text = text.trim().replace(/[^\S\n]+/g, ' ');

					const lines = text.split('\n');
					let cleanedText = '';
					const startLine = Math.max(0, lines.length - 10);
					for (let i = lines.length - 1; i >= startLine; i--) {
						if (!delTextsOfEnd.some(navigationText => lines[i].trim().includes(navigationText))) {
							cleanedText = lines[i] + (cleanedText ? '\n' + cleanedText : '');
						}
					}
					cleanedText = lines.slice(0, startLine).join('\n') + (cleanedText ? '\n' + cleanedText : '');
					return cleanedText.trim();
				}

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
				text = text.trim().replace(/[^\S\n]+/g, ' ');

				const lines = text.split('\n');
				let cleanedText = '';
				const startLine = Math.max(0, lines.length - 10);
				for (let i = lines.length - 1; i >= startLine; i--) {
					if (!delTextsOfEnd.some(navigationText => lines[i].trim().includes(navigationText))) {
						cleanedText = lines[i] + (cleanedText ? '\n' + cleanedText : '');
					}
				}
				cleanedText = lines.slice(0, startLine).join('\n') + (cleanedText ? '\n' + cleanedText : '');
				return cleanedText.trim();
			}
			getChapterContent()
		`)
		if err != nil {
			log.Printf("获取章节内容失败: %v, 跳过...", err)
			errorMsg := fmt.Sprintf("【错误】获取章节内容失败: %s (URL: %s)\n\n", currentChapterTitle, currentChapterURL)
			file.WriteString(errorMsg)
			goto NextChapter
		}

		if currentChapterBaseTitle == extractBaseChapterTitle(currentChapterTitle) && currentPageNum > 1 {
			_, err = file.WriteString(fmt.Sprintf("%s\n\n", chapterContent.(string)))
		} else {
			_, err = file.WriteString(fmt.Sprintf("%s\n\n%s\n\n", currentChapterTitle, chapterContent.(string)))
		}
		if err != nil {
			log.Printf("写入章节内容失败: %v", err)
		}

	NextChapter:
		var nextChapterURL string

		// 模拟人类滚动
		_, _ = page.Evaluate(`
			function humanScrollToBottom() {
				const duration = 2000 + Math.random() * 3000;
				const startTime = Date.now();
				const startScroll = window.scrollY;
				const endScroll = document.body.scrollHeight - window.innerHeight;
				const distance = endScroll - startScroll;

				function easeInOutCubic(t) {
					return t < 0.5 ? 4 * t * t * t : (t - 1) * (2 * t - 2) * (2 * t - 2) + 1;
				}
				function addRandomness(progress) {
					const randomFactor = 1 + (Math.random() - 0.5) * 0.2;
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
				scrollStep();
				return new Promise(resolve => setTimeout(resolve, duration + 500));
			}
			humanScrollToBottom()
		`)

		nextLink, err := page.Evaluate(`
			function findNextChapter() {
				const nextChapterKeywords = ['下一章', '下一页', '下节', '下一话', '下一回'];
				let link = null;
				for (const keyword of nextChapterKeywords) {
					link = Array.from(document.querySelectorAll('a')).find(a =>
						a.textContent.includes(keyword) && a.href);
					if (link) break;
				}
				if (!link) {
					link = document.querySelector('a[id*="next" i], a[class*="next" i]');
				}
				if (!link) {
					const currentChapterText = document.title || '';
					const chapterNumMatch = currentChapterText.match(/第(\d+)[章节回]/);
					let nextChapterNum = 0;
					if (chapterNumMatch && chapterNumMatch[1]) {
						nextChapterNum = parseInt(chapterNumMatch[1]) + 1;
					} else {
						nextChapterNum = 1000;
					}
					const nextChapterPattern = new RegExp('第' + nextChapterNum + '[章节回]');
					link = Array.from(document.querySelectorAll('a')).find(a =>
						nextChapterPattern.test(a.textContent));
				}
				if (!link) {
					link = document.querySelector('a[rel="next"]');
				}
				if (link) {
					const href = link.href;
					const text = link.textContent.trim();
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
		`)
		if err == nil && nextLink != nil && nextLink.(string) != "" {
			nextLinkStr := nextLink.(string)
			absURL, err := net_url.Parse(nextLinkStr)
			if err == nil {
				baseURL, _ := net_url.Parse(currentChapterURL)
				nextChapterURL = baseURL.ResolveReference(absURL).String()
			}
		}

		if nextChapterURL == "" && len(chapterList) > 0 && currentChapterIndex < len(chapterList) {
			nextChapterURL = chapterList[currentChapterIndex].Href
			fmt.Printf("使用目录页的链接作为下一章: %s\n", nextChapterURL)
		}

		if nextChapterURL == "" {
			fmt.Println("未找到下一章链接，下载完成")
			break
		}

		if totalChapterCount > 0 && currentChapterIndex >= totalChapterCount {
			fmt.Println("已下载所有章节，下载完成")
			break
		}

		nextLinkText, err = page.Evaluate(`
			function getNextLinkText() {
				const paginationElements = document.querySelectorAll(
					'.pagination, .pager, [id*="page"], [class*="page"], [role="navigation"], a[href*="page="], a[href*="p="]'
				);
				for (let i = 0; i < paginationElements.length; i++) {
					const text = paginationElements[i].textContent;
					if (text.includes('第') && (text.includes('页') || text.includes('/'))) {
						return text;
					}
				}
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
		`)
		if err == nil && nextLinkText != nil {
			text := nextLinkText.(string)
			pageMatch := regexp.MustCompile(`第(\d+)[页\/]`).FindStringSubmatch(text)
			if len(pageMatch) > 1 {
				pageNumFromNav, _ := strconv.Atoi(pageMatch[1])
				if pageNumFromNav > currentPageNum {
					currentPageNum = pageNumFromNav
				}
			}
			if strings.Contains(text, "下一页") || strings.Contains(text, "页") {
				currentPageNum++
			} else {
				currentPageNum = 1
				currentChapterIndex++
			}
		} else {
			currentChapterIndex++
			currentPageNum = 1
		}

		currentChapterURL = nextChapterURL

		randomDelay := time.Duration(5+rand.Intn(56)) * time.Second
		if nextLinkText != nil && strings.Contains(nextLinkText.(string), "下一页") {
			fmt.Printf("等待 %v 后下载下一页...\n", randomDelay)
		} else {
			fmt.Printf("等待 %v 后下载下一章...\n", randomDelay)
		}
		time.Sleep(randomDelay)
	}

	fmt.Printf("小说下载完成，保存至: %s\n", fileName)
	return nil
}

// 提取章节的基础标题，去除分页信息
func extractBaseChapterTitle(title string) string {
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
		baseTitle = strings.TrimSpace(baseTitle)
	}
	return baseTitle
}

// 清理文件名
func cleanFileName(name string) string {
	invalidChars := regexp.MustCompile(`[<>:"/\|?*]`)
	cleaned := invalidChars.ReplaceAllString(name, "_")
	cleaned = regexp.MustCompile(`_+`).ReplaceAllString(cleaned, "_")
	cleaned = strings.Trim(cleaned, "_")
	return cleaned
}

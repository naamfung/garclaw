package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

var (
	isAlpine    = false
	isWindows   = false
	browserPath = ""
)

func init() {
	// 检测是否为视窗系统
	isWindows = os.PathSeparator == '\\'

	// 检测是否为 Alpine Linux 系统
	osRelease, err := os.ReadFile("/etc/os-release")
	if err == nil {
		if strings.Contains(string(osRelease), "Alpine") {
			isAlpine = true
		}
	}

	// --- 关键点1: 安装时跳过浏览器下载 ---
	// 通过 RunOptions 告诉 Playwright 的驱动安装程序，不要自动下载浏览器。
	// 我们必须自己保证系统里经已有可用的 Chromium。
	installOptions := &playwright.RunOptions{
		SkipInstallBrowsers: true, // 核心参数：跳过浏览器二进制文件的下载
	}

	if isAlpine || isWindows {
		// 执行安装（主要是安装驱动程序，浏览器我们手动管理）
		err = playwright.Install(installOptions)
		if err != nil {
			if isAlpine {
				log.Printf("Alpine Linux 系统安装 Playwright 驱动失败: %v", err)
			} else {
				log.Printf("视窗系统安装 Playwright 驱动失败: %v", err)
			}
		}
	} else {
		err = playwright.Install() // 其他系统安装驱动程序无须特殊处理
		if err != nil {
			log.Printf("安装 Playwright 驱动失败: %v", err)
		}
	}

	// 检查是否已安装浏览器
	browserPaths := []string{"chromium", "chromium-browser", "google-chrome"}
	hasBrowser := false

	// 首先尝试在系统 PATH 中查找
	for _, browser := range browserPaths {
		if _, err := exec.LookPath(browser); err == nil {
			hasBrowser = true
			break
		}
	}

	// 如果在视窗系统上，直接检查常见的浏览器安装路径
	if isWindows && !hasBrowser {
		// 生成所有可能的磁盘盘符（A-Z）
		driveLetters := make([]string, 0, 26)
		for i := 'A'; i <= 'Z'; i++ {
			driveLetters = append(driveLetters, string(i))
		}

		// 定义基础路径模板
		basePaths := []string{
			"Program Files/Google/Chrome/Application/chrome.exe",
			"Program Files (x86)/Google/Chrome/Application/chrome.exe",
			"Users/" + os.Getenv("USERNAME") + "/AppData/Local/Google/Chrome/Application/chrome.exe",
			"Users/" + os.Getenv("USERNAME") + "/AppData/Local/Chromium/Application/chrome.exe",
			"Program Files/Microsoft/Edge/Application/msedge.exe",
			"Program Files (x86)/Microsoft/Edge/Application/msedge.exe",
			"Program Files/Mozilla Firefox/firefox.exe",
			"Program Files (x86)/Mozilla Firefox/firefox.exe",
		}

		// 检查所有磁盘盘符的所有基础路径
		for _, drive := range driveLetters {
			for _, basePath := range basePaths {
				fullPath := drive + ":/" + basePath
				if _, err := os.Stat(fullPath); err == nil {
					hasBrowser = true
					browserPath = fullPath
					break
				}
			}
			if hasBrowser {
				break
			}
		}
	}
	if !hasBrowser {
		log.Println("未检测到浏览器，请确保已安装 Chromium")
		return
	}

	// 在 Alpine Linux 或视窗系统上，使用系统安装的浏览器
	if isAlpine {
		log.Println("在 Alpine Linux 上使用系统安装的 Chromium")
	} else if isWindows {
		log.Println("在视窗系统上使用系统安装的浏览器")
	}
}

// 启动浏览器并创建页面
func launchBrowser() (*playwright.Playwright, playwright.Browser, playwright.Page, error) {
	// 启动 Playwright
	pw, err := playwright.Run(&playwright.RunOptions{
		SkipInstallBrowsers: true,
		Verbose:             false,
	})
	if err != nil {
		log.Printf("启动 Playwright 失败: %v", err)
		return nil, nil, nil, err
	}

	var browser playwright.Browser

	// 使用 "Channel" 指向系统安装的 Chromium
	launchOptions := playwright.BrowserTypeLaunchOptions{
		Channel:  playwright.String("chrome"), // 告诉 Playwright 启动系统 Chrome/Chromium
		Headless: playwright.Bool(true),
		Args:     []string{"--no-sandbox", "--disable-setuid-sandbox", "--disable-dev-shm-usage"},
	}

	// 启动浏览器
	browser, err = pw.Chromium.Launch(launchOptions)
	if err != nil {
		if isDebug {
			log.Printf("使用 Channel 方式启动浏览器失败: %v", err)
		}
		// 如果有浏览器路径，尝试使用指定路径的方式启动
		if browserPath != "" {
			if isDebug {
				log.Printf("尝试使用指定路径启动浏览器: %s", browserPath)
			}
			launchOptionsWithPath := playwright.BrowserTypeLaunchOptions{
				ExecutablePath: playwright.String(browserPath),
				Headless:       playwright.Bool(true),
				Args:           []string{"--no-sandbox", "--disable-setuid-sandbox", "--disable-dev-shm-usage"},
			}
			browser, err = pw.Chromium.Launch(launchOptionsWithPath)
			if err != nil {
				log.Printf("使用指定路径启动浏览器失败: %v", err)
				pw.Stop()
				return nil, nil, nil, err
			}
		} else {
			pw.Stop()
			return nil, nil, nil, err
		}
	}

	page, err := browser.NewPage()
	if err != nil {
		log.Printf("创建页面失败: %v", err)
		browser.Close()
		pw.Stop()
		return nil, nil, nil, err
	}

	return pw, browser, page, nil
}

// 搜索结果结构
type SearchResult struct {
	Title string `json:"title"`
	Link  string `json:"link"`
}

// 搜索功能
func Search(keyword string) ([]SearchResult, error) {
	// 启动浏览器并创建页面
	pw, browser, page, err := launchBrowser()
	if err != nil {
		return nil, err
	}
	defer pw.Stop()
	defer browser.Close()
	defer page.Close()

	// 移除超时设置，使用 Playwright 自带的等待机制
	ctx := context.Background()

	searchURL := fmt.Sprintf("https://www.baidu.com/s?ie=UTF-8&wd=%s", keyword)
	return search(ctx, page, searchURL)
}

// 访问功能
func Visit(url string) (string, error) {
	// 启动浏览器并创建页面
	pw, browser, page, err := launchBrowser()
	if err != nil {
		return "", err
	}
	defer pw.Stop()
	defer browser.Close()
	defer page.Close()

	// 移除超时设置，使用 Playwright 自带的等待机制
	ctx := context.Background()

	return visitURL(ctx, page, url)
}

// 通用下载功能
func Download(url string) (string, error) {
	// 启动浏览器并创建页面
	pw, browser, page, err := launchBrowser()
	if err != nil {
		return "", err
	}
	defer pw.Stop()
	defer browser.Close()
	defer page.Close()

	// 移除超时设置，使用 Playwright 自带的等待机制

	timeout := float64(5 * time.Minute / time.Millisecond)
	if _, err = page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   &timeout,
	}); err != nil {
		log.Printf("导航失败: %v", err)
		return "", err
	}

	if err := page.Locator("body").WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateAttached,
	}); err != nil {
		log.Printf("等待 body 失败: %v", err)
		return "", err
	}

	time.Sleep(3 * time.Second)

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

	timeout := float64(60 * time.Second / time.Millisecond)
	if _, err := page.Goto(searchURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   &timeout,
	}); err != nil {
		log.Printf("导航到搜索页失败: %v", err)
		return nil, err
	}

	if err := page.Locator("#content_left").WaitFor(playwright.LocatorWaitForOptions{
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

	timeout := float64(60 * time.Second / time.Millisecond)
	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   &timeout,
	}); err != nil {
		log.Printf("导航到页面失败: %v", err)
		return "", err
	}

	if err := page.Locator("body").WaitFor(playwright.LocatorWaitForOptions{
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

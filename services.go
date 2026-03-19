package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
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
	isWindows = runtime.GOOS == "windows"

	// 检测是否为 Alpine Linux
	if !isWindows {
		osRelease, err := os.ReadFile("/etc/os-release")
		if err == nil {
			if strings.Contains(string(osRelease), "Alpine") {
				isAlpine = true
			}
		}
	}

	// 安装 Playwright 驱动（跳过浏览器下载）
	installOptions := &playwright.RunOptions{
		SkipInstallBrowsers: true,
	}
	err := playwright.Install(installOptions)
	if err != nil {
		log.Printf("安装 Playwright 驱动失败: %v", err)
	}

	// 检测系统是否已有可用的 Chromium 浏览器
	detectBrowser()
}

// detectBrowser 尝试查找系统安装的 Chromium/Chrome
func detectBrowser() {
	// 常见浏览器可执行文件名称
	browserNames := []string{
		"chromium", "chromium-browser", "google-chrome", "chrome", "brave-browser",
		"microsoft-edge", "edge",
	}
	// 常见安装路径（非 Windows）
	commonPaths := []string{
		"/usr/bin/",
		"/usr/local/bin/",
		"/opt/google/chrome/",
		"/opt/chromium/",
	}

	// 先在 PATH 中查找
	for _, name := range browserNames {
		if path, err := exec.LookPath(name); err == nil {
			browserPath = path
			log.Printf("找到浏览器: %s", path)
			return
		}
	}

	// 在常见路径中查找
	if !isWindows {
		for _, dir := range commonPaths {
			for _, name := range browserNames {
				fullPath := dir + name
				if _, err := os.Stat(fullPath); err == nil {
					browserPath = fullPath
					log.Printf("找到浏览器: %s", fullPath)
					return
				}
			}
		}
	} else {
		// Windows 上的查找邏輯（略，原有代碼已處理）
		driveLetters := make([]string, 0, 26)
		for i := 'A'; i <= 'Z'; i++ {
			driveLetters = append(driveLetters, string(i))
		}
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
		for _, drive := range driveLetters {
			for _, basePath := range basePaths {
				fullPath := drive + ":/" + basePath
				if _, err := os.Stat(fullPath); err == nil {
					browserPath = fullPath
					log.Printf("找到浏览器: %s", fullPath)
					return
				}
			}
		}
	}

	if browserPath == "" {
		log.Println("警告：未找到 Chromium 或 Chrome 浏览器，网页功能将不可用")
	}
}

// 启动浏览器并创建页面
func launchBrowser() (*playwright.Playwright, playwright.Browser, playwright.Page, error) {
	pw, err := playwright.Run(&playwright.RunOptions{
		SkipInstallBrowsers: true,
		Verbose:             false,
	})
	if err != nil {
		log.Printf("启动 Playwright 失败: %v", err)
		return nil, nil, nil, err
	}

	var browser playwright.Browser
	launchOptions := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
		Args:     []string{"--no-sandbox", "--disable-setuid-sandbox", "--disable-dev-shm-usage"},
	}

	// 先嘗試用 channel 啟動 Chrome
	if !isAlpine && !isWindows {
		launchOptions.Channel = playwright.String("chrome")
		browser, err = pw.Chromium.Launch(launchOptions)
		if err == nil {
			goto CREATE_PAGE
		}
	}

	// 如果 channel 失敗且我們有瀏覽器路徑，則使用指定路徑
	if browserPath != "" {
		launchOptions.ExecutablePath = playwright.String(browserPath)
		browser, err = pw.Chromium.Launch(launchOptions)
		if err == nil {
			goto CREATE_PAGE
		}
	}

	// 最後嘗試默認方式
	launchOptions.Channel = nil
	launchOptions.ExecutablePath = nil
	browser, err = pw.Chromium.Launch(launchOptions)
	if err != nil {
		log.Printf("啟動瀏覽器失敗: %v", err)
		pw.Stop()
		return nil, nil, nil, err
	}

CREATE_PAGE:
	page, err := browser.NewPage()
	if err != nil {
		log.Printf("創建頁面失敗: %v", err)
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
	pw, browser, page, err := launchBrowser()
	if err != nil {
		return nil, err
	}
	defer pw.Stop()
	defer browser.Close()
	defer page.Close()

	searchURL := fmt.Sprintf("https://www.baidu.com/s?ie=UTF-8&wd=%s", keyword)
	return search(page, searchURL)
}

// 访问功能
func Visit(url string) (string, error) {
	pw, browser, page, err := launchBrowser()
	if err != nil {
		return "", err
	}
	defer pw.Stop()
	defer browser.Close()
	defer page.Close()

	return visitURL(page, url)
}

// 通用下载功能
func Download(url string) (string, error) {
	pw, browser, page, err := launchBrowser()
	if err != nil {
		return "", err
	}
	defer pw.Stop()
	defer browser.Close()
	defer page.Close()

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
func search(page playwright.Page, searchURL string) ([]SearchResult, error) {
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
func visitURL(page playwright.Page, url string) (string, error) {
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
	pageText = strings.TrimPrefix(pageText, "You need to enable JavaScript to run this app.")

	if len(pageText) > 512 {
		fmt.Println("Page content (truncated): " + pageText[:512] + "...")
	} else {
		fmt.Println(pageText)
	}
	return pageText, nil
}


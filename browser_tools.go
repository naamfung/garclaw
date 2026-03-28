package main

import (
        "context"
        "crypto/md5"
        "encoding/base64"
        "encoding/json"
        "fmt"
        "log"
        "os"
        "path/filepath"
        "strings"
        "time"

        "github.com/go-rod/rod/lib/input"
        "github.com/go-rod/rod/lib/proto"
)

// ============================================================
// 浏览器工具增强模块
// 提供更强大的浏览器自动化能力

// getBrowserTimeout 获取浏览器超时时间（秒）
// 如果传入的 timeoutSec > 0，使用传入值；否则使用全局配置
func getBrowserTimeout(timeoutSec int) int {
        if timeoutSec > 0 {
                return timeoutSec
        }
        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = 30
        }
        return timeout
}
// ============================================================

// browserSafeOp 执行浏览器操作，捕获 panic 并转换为 error
func browserSafeOp(op string, fn func() error) (err error) {
        defer func() {
                if r := recover(); r != nil {
                        errStr := fmt.Sprintf("%v", r)
                        isTimeout := strings.Contains(errStr, "context deadline exceeded")
                        
                        if isTimeout {
                                timeout := globalTimeoutConfig.Browser
                                if timeout <= 0 {
                                        timeout = DefaultBrowserTimeout
                                }
                                err = &BrowserError{Op: op, Err: fmt.Errorf("%v", r), Timeout: timeout}
                        } else {
                                err = &BrowserError{Op: op, Err: fmt.Errorf("%v", r)}
                        }
                }
        }()
        return fn()
}

// ============================================================
// 交互操作类工具
// ============================================================

// BrowserClickResult 点击操作结果
type BrowserClickResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
        URL     string `json:"url,omitempty"` // 点击后的 URL
}

// BrowserClick 点击页面元素
// selector: CSS 选择器，如 "button.submit", "#login-btn", "a[href*='detail']"
// timeoutSec: 可选超时时间（秒），0 或负数使用默认值
func BrowserClick(url, selector string, timeoutSec int) (result *BrowserClickResult, err error) {
        err = browserSafeOp("BrowserClick", func() error {
                result, err = browserClickImpl(url, selector, timeoutSec)
                return err
        })
        return
}

func browserClickImpl(url, selector string, timeoutSec int) (*BrowserClickResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := getBrowserTimeout(timeoutSec)
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage()
        page = page.Context(ctx)

        // 导航到页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()
        if ctx.Err() != nil {
                return nil, fmt.Errorf("浏览器操作超时")
        }

        // 等待元素出现
        element, err := page.Element(selector)
        if err != nil {
                return nil, fmt.Errorf("未找到元素 '%s': %w", selector, err)
        }

        // 滚动到元素可见
        if err := element.ScrollIntoView(); err != nil {
                log.Printf("滚动到元素失败: %v", err)
        }

        // 点击元素 - 使用 MustClick
        element.MustClick()

        // 等待可能的页面变化
        time.Sleep(500 * time.Millisecond)

        // 获取当前 URL
        info, _ := page.Info()

        return &BrowserClickResult{
                Success: true,
                Message: fmt.Sprintf("成功点击元素: %s", selector),
                URL:     info.URL,
        }, nil
}

// BrowserTypeResult 输入操作结果
type BrowserTypeResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
        Value   string `json:"value,omitempty"`
}

// BrowserType 在输入框中输入文本
// selector: CSS 选择器，如 "input[name='username']", "#search-box"
// text: 要输入的文本
// submit: 是否按回车提交
func BrowserType(url, selector, text string, submit bool, timeoutSec int) (result *BrowserTypeResult, err error) {
        err = browserSafeOp("BrowserType", func() error {
                result, err = browserTypeImpl(url, selector, text, submit, timeoutSec)
                return err
        })
        return
}

func browserTypeImpl(url, selector, text string, submit bool, timeoutSec int) (*BrowserTypeResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := getBrowserTimeout(timeoutSec)
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage()
        page = page.Context(ctx)

        // 导航到页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()
        if ctx.Err() != nil {
                return nil, fmt.Errorf("浏览器操作超时")
        }

        // 查找输入框
        element, err := page.Element(selector)
        if err != nil {
                return nil, fmt.Errorf("未找到输入框 '%s': %w", selector, err)
        }

        // 点击获取焦点
        element.MustClick()

        // 清空并输入文本
        element.SelectAllText()
        element.Input(text)

        // 可选：按回车提交
        if submit {
                page.Keyboard.Press(input.Enter)
                time.Sleep(500 * time.Millisecond)
        }

        return &BrowserTypeResult{
                Success: true,
                Message: fmt.Sprintf("成功输入文本到: %s", selector),
                Value:   text,
        }, nil
}

// BrowserScrollResult 滚动操作结果
type BrowserScrollResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
        Height  int    `json:"height,omitempty"` // 页面总高度
}

// BrowserScroll 滚动页面
// direction: "up" 或 "down"
// amount: 滚动像素数，如 500
func BrowserScroll(url, direction string, amount int, timeoutSec int) (result *BrowserScrollResult, err error) {
        err = browserSafeOp("BrowserScroll", func() error {
                result, err = browserScrollImpl(url, direction, amount, timeoutSec)
                return err
        })
        return
}

func browserScrollImpl(url, direction string, amount int, timeoutSec int) (*BrowserScrollResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := getBrowserTimeout(timeoutSec)
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage()
        page = page.Context(ctx)

        // 导航到页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()
        if ctx.Err() != nil {
                return nil, fmt.Errorf("浏览器操作超时")
        }

        // 执行滚动
        scrollJS := ""
        if direction == "up" {
                scrollJS = fmt.Sprintf("window.scrollBy(0, -%d)", amount)
        } else {
                scrollJS = fmt.Sprintf("window.scrollBy(0, %d)", amount)
        }

        page.MustEval(scrollJS)

        // 获取页面高度
        height := page.MustEval("() => document.body.scrollHeight").Int()

        return &BrowserScrollResult{
                Success: true,
                Message: fmt.Sprintf("成功向%s滚动 %d 像素", direction, amount),
                Height:  height,
        }, nil
}

// ============================================================
// 等待操作类工具
// ============================================================

// BrowserWaitResult 等待操作结果
type BrowserWaitResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
}

// BrowserWaitElement 等待元素出现
// selector: CSS 选择器
// waitTimeout: 等待超时秒数
func BrowserWaitElement(url, selector string, waitTimeout int) (result *BrowserWaitResult, err error) {
        err = browserSafeOp("BrowserWaitElement", func() error {
                result, err = browserWaitElementImpl(url, selector, waitTimeout)
                return err
        })
        return
}

func browserWaitElementImpl(url, selector string, waitTimeout int) (*BrowserWaitResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        if waitTimeout <= 0 {
                waitTimeout = 10
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = 30
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage()
        page = page.Context(ctx)

        // 导航到页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        // 使用独立的超时等待元素
        waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Duration(waitTimeout)*time.Second)
        defer waitCancel()

        // 创建带超时的页面
        waitPage := page.Context(waitCtx)

        // 等待元素
        _, err = waitPage.Element(selector)
        if err != nil {
                return nil, fmt.Errorf("等待元素 '%s' 超时: %w", selector, err)
        }

        return &BrowserWaitResult{
                Success: true,
                Message: fmt.Sprintf("元素 '%s' 已出现", selector),
        }, nil
}

// BrowserWaitIdle 等待页面网络空闲
// waitTimeout: 最长等待秒数
func BrowserWaitIdle(url string, waitTimeout int) (result *BrowserWaitResult, err error) {
        err = browserSafeOp("BrowserWaitIdle", func() error {
                result, err = browserWaitIdleImpl(url, waitTimeout)
                return err
        })
        return
}

func browserWaitIdleImpl(url string, waitTimeout int) (*BrowserWaitResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        if waitTimeout <= 0 {
                waitTimeout = 10
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = 30
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage()
        page = page.Context(ctx)

        // 导航到页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        // 等待网络空闲
        waitFunc := page.MustWaitRequestIdle()
        waitFunc()

        // 额外等待 DOM 稳定
        if err := page.WaitDOMStable(time.Duration(waitTimeout)*time.Second, 0.1); err != nil {
                log.Printf("DOM 稳定等待: %v", err)
        }

        return &BrowserWaitResult{
                Success: true,
                Message: "页面已加载完成且网络空闲",
        }, nil
}

// ============================================================
// 内容提取类工具
// ============================================================

// LinkInfo 链接信息
type LinkInfo struct {
        Text string `json:"text"`
        Href string `json:"href"`
}

// BrowserExtractLinksResult 链接提取结果
type BrowserExtractLinksResult struct {
        URL   string     `json:"url"`
        Count int        `json:"count"`
        Links []LinkInfo `json:"links"`
}

// BrowserExtractLinks 提取页面所有链接
func BrowserExtractLinks(url string, timeoutSec int) (result *BrowserExtractLinksResult, err error) {
        err = browserSafeOp("BrowserExtractLinks", func() error {
                result, err = browserExtractLinksImpl(url, timeoutSec)
                return err
        })
        return
}

func browserExtractLinksImpl(url string, timeoutSec int) (*BrowserExtractLinksResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := getBrowserTimeout(timeoutSec)
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage()
        page = page.Context(ctx)

        // 导航到页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()
        if ctx.Err() != nil {
                return nil, fmt.Errorf("浏览器操作超时")
        }

        // 提取所有链接
        linksJSON := page.MustEval(`() => {
                return JSON.stringify(Array.from(document.querySelectorAll('a')).map(a => ({
                        text: a.innerText.trim(),
                        href: a.href
                })).filter(l => l.href && l.href.startsWith('http')));
        }`).Str()

        var links []LinkInfo
        if err := json.Unmarshal([]byte(linksJSON), &links); err != nil {
                // 如果 JSON 解析失败，尝试简单解析
                links = parseSimpleLinks(linksJSON)
        }

        return &BrowserExtractLinksResult{
                URL:   url,
                Count: len(links),
                Links: links,
        }, nil
}

// parseSimpleLinks 简单解析链接（备用）
func parseSimpleLinks(jsonStr string) []LinkInfo {
        var links []LinkInfo
        // 移除首尾的方括号
        jsonStr = strings.Trim(jsonStr, "[]")
        if jsonStr == "" {
                return links
        }

        // 简单分割处理
        parts := strings.Split(jsonStr, "},{")
        for _, part := range parts {
                part = strings.Trim(part, "{}")
                link := LinkInfo{}
                // 提取 text 和 href
                if strings.Contains(part, `"text"`) {
                        textStart := strings.Index(part, `"text"`)
                        if textStart != -1 {
                                rest := part[textStart+7:]
                                if strings.Contains(rest, `:"`) {
                                        valStart := strings.Index(rest, `:"`)
                                        if valStart != -1 {
                                                val := rest[valStart+2:]
                                                if end := strings.Index(val, `"`); end != -1 {
                                                        link.Text = val[:end]
                                                }
                                        }
                                }
                        }
                }
                if strings.Contains(part, `"href"`) {
                        hrefStart := strings.Index(part, `"href"`)
                        if hrefStart != -1 {
                                rest := part[hrefStart+7:]
                                if strings.Contains(rest, `:"`) {
                                        valStart := strings.Index(rest, `:"`)
                                        if valStart != -1 {
                                                val := rest[valStart+2:]
                                                if end := strings.Index(val, `"`); end != -1 {
                                                        link.Href = val[:end]
                                                }
                                        }
                                }
                        }
                }
                if link.Href != "" {
                        links = append(links, link)
                }
        }
        return links
}

// ImageInfo 图片信息
type ImageInfo struct {
        Src string `json:"src"`
        Alt string `json:"alt"`
}

// BrowserExtractImagesResult 图片提取结果
type BrowserExtractImagesResult struct {
        URL    string      `json:"url"`
        Count  int         `json:"count"`
        Images []ImageInfo `json:"images"`
}

// BrowserExtractImages 提取页面所有图片
func BrowserExtractImages(url string, timeoutSec int) (result *BrowserExtractImagesResult, err error) {
        err = browserSafeOp("BrowserExtractImages", func() error {
                result, err = browserExtractImagesImpl(url, timeoutSec)
                return err
        })
        return
}

func browserExtractImagesImpl(url string, timeoutSec int) (*BrowserExtractImagesResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := getBrowserTimeout(timeoutSec)
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage()
        page = page.Context(ctx)

        // 导航到页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()
        if ctx.Err() != nil {
                return nil, fmt.Errorf("浏览器操作超时")
        }

        // 提取所有图片
        imagesJSON := page.MustEval(`() => {
                return JSON.stringify(Array.from(document.querySelectorAll('img')).map(img => ({
                        src: img.src,
                        alt: img.alt || ''
                })).filter(i => i.src && i.src.startsWith('http')));
        }`).Str()

        var images []ImageInfo
        if err := json.Unmarshal([]byte(imagesJSON), &images); err != nil {
                log.Printf("解析图片列表失败: %v", err)
                images = []ImageInfo{}
        }

        return &BrowserExtractImagesResult{
                URL:    url,
                Count:  len(images),
                Images: images,
        }, nil
}

// ElementInfo 元素信息
type ElementInfo struct {
        Tag     string            `json:"tag"`
        Text    string            `json:"text"`
        HTML    string            `json:"html,omitempty"`
        Attribs map[string]string `json:"attribs,omitempty"`
}

// BrowserExtractElementsResult 元素提取结果
type BrowserExtractElementsResult struct {
        URL      string        `json:"url"`
        Selector string        `json:"selector"`
        Count    int           `json:"count"`
        Elements []ElementInfo `json:"elements"`
}

// BrowserExtractElements 提取指定选择器的所有元素
// selector: CSS 选择器，如 ".article", "div.content p", "h2.title"
// includeHTML: 是否包含 HTML 内容
func BrowserExtractElements(url, selector string, includeHTML bool, timeoutSec int) (result *BrowserExtractElementsResult, err error) {
        err = browserSafeOp("BrowserExtractElements", func() error {
                result, err = browserExtractElementsImpl(url, selector, includeHTML, timeoutSec)
                return err
        })
        return
}

func browserExtractElementsImpl(url, selector string, includeHTML bool, timeoutSec int) (*BrowserExtractElementsResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := getBrowserTimeout(timeoutSec)
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage()
        page = page.Context(ctx)

        // 导航到页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()
        if ctx.Err() != nil {
                return nil, fmt.Errorf("浏览器操作超时")
        }

        // 构建提取脚本
        includeHTMLStr := "false"
        if includeHTML {
                includeHTMLStr = "true"
        }
        // 转义选择器中的特殊字符，防止 JavaScript 注入
        escapedSelector := selector
        escapedSelector = strings.ReplaceAll(escapedSelector, `\`, `\\`)
        escapedSelector = strings.ReplaceAll(escapedSelector, `"`, `\"`)
        script := `() => {
                const selector = "` + escapedSelector + `";
                const includeHTML = ` + includeHTMLStr + `;
                return JSON.stringify(Array.from(document.querySelectorAll(selector)).map(el => ({
                        tag: el.tagName.toLowerCase(),
                        text: el.innerText.trim(),
                        html: includeHTML ? el.innerHTML : '',
                        attribs: Array.from(el.attributes).reduce((acc, attr) => {
                                acc[attr.name] = attr.value;
                                return acc;
                        }, {})
                })));
        }`

        elementsJSON := page.MustEval(script).Str()

        var elements []ElementInfo
        if err := json.Unmarshal([]byte(elementsJSON), &elements); err != nil {
                log.Printf("解析元素列表失败: %v", err)
                elements = []ElementInfo{}
        }

        return &BrowserExtractElementsResult{
                URL:      url,
                Selector: selector,
                Count:    len(elements),
                Elements: elements,
        }, nil
}

// ============================================================
// 高级功能类工具
// ============================================================

// BrowserScreenshotResult 截图结果
type BrowserScreenshotResult struct {
        URL       string `json:"url"`
        Success   bool   `json:"success"`
        SavedFile string `json:"savedFile,omitempty"` // 截图保存的文件路径
        FullPage  bool   `json:"fullPage"`
        Width     int    `json:"width"`
        Height    int    `json:"height"`
        Size      int64  `json:"size"` // 文件大小（字节）
}

// BrowserScreenshot 页面截图
// fullPage: 是否截取整个页面（包括滚动区域）
func BrowserScreenshot(url string, fullPage bool, timeoutSec int) (result *BrowserScreenshotResult, err error) {
        err = browserSafeOp("BrowserScreenshot", func() error {
                result, err = browserScreenshotImpl(url, fullPage, timeoutSec)
                return err
        })
        return
}

func browserScreenshotImpl(url string, fullPage bool, timeoutSec int) (*BrowserScreenshotResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := getBrowserTimeout(timeoutSec)
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage()
        page = page.Context(ctx)

        // 导航到页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()
        if ctx.Err() != nil {
                return nil, fmt.Errorf("浏览器操作超时")
        }

        // 等待页面稳定
        time.Sleep(1 * time.Second)

        // 获取页面尺寸
        width := page.MustEval("() => window.innerWidth").Int()
        height := page.MustEval("() => document.body.scrollHeight").Int()

        // 截图
        var screenshot []byte
        if fullPage {
                screenshot = page.MustScreenshotFullPage()
        } else {
                screenshot = page.MustScreenshot()
        }

        // 保存截图到文件，而不是返回 base64（基于程序所在目录）
        downloadDir := filepath.Join(globalExecDir, "download")
        if err := os.MkdirAll(downloadDir, 0755); err != nil {
                return nil, fmt.Errorf("创建下载目录失败: %w", err)
        }

        // 生成文件名
        timestamp := time.Now().Format("20060102_150405")
        hash := md5.Sum([]byte(url))
        urlHash := fmt.Sprintf("%x", hash)[:8]
        fileName := fmt.Sprintf("screenshot_%s_%s.png", timestamp, urlHash)
        filePath := filepath.Join(downloadDir, fileName)

        // 写入文件
        if err := os.WriteFile(filePath, screenshot, 0644); err != nil {
                return nil, fmt.Errorf("保存截图失败: %w", err)
        }

        fmt.Println("Screenshot saved to: " + filePath)

        return &BrowserScreenshotResult{
                URL:       url,
                Success:   true,
                SavedFile: filePath,
                FullPage:  fullPage,
                Width:     width,
                Height:    height,
                Size:      int64(len(screenshot)),
        }, nil
}

// BrowserExecuteJSResult JS 执行结果
type BrowserExecuteJSResult struct {
        URL     string      `json:"url"`
        Success bool        `json:"success"`
        Result  interface{} `json:"result"`
}

// BrowserExecuteJS 执行自定义 JavaScript
// script: JavaScript 代码，如 "() => document.title" 或 "() => { return {url: location.href, title: document.title}; }"
func BrowserExecuteJS(url, script string, timeoutSec int) (result *BrowserExecuteJSResult, err error) {
        err = browserSafeOp("BrowserExecuteJS", func() error {
                result, err = browserExecuteJSImpl(url, script, timeoutSec)
                return err
        })
        return
}

func browserExecuteJSImpl(url, script string, timeoutSec int) (*BrowserExecuteJSResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := getBrowserTimeout(timeoutSec)
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage()
        page = page.Context(ctx)

        // 导航到页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()
        if ctx.Err() != nil {
                return nil, fmt.Errorf("浏览器操作超时")
        }

        // 执行 JavaScript
        result := page.MustEval(script).Str()

        return &BrowserExecuteJSResult{
                URL:     url,
                Success: true,
                Result:  result,
        }, nil
}

// FormData 表单数据
type FormData map[string]string

// BrowserFillFormResult 表单填写结果
type BrowserFillFormResult struct {
        URL      string `json:"url"`
        Success  bool   `json:"success"`
        Message  string `json:"message"`
        FinalURL string `json:"finalUrl,omitempty"`
}

// BrowserFillForm 填写并提交表单
// formData: 字段名 -> 值的映射，如 {"username": "admin", "password": "123456"}
// submitSelector: 提交按钮选择器，如 "button[type='submit']"，为空则按回车
func BrowserFillForm(url string, formData FormData, submitSelector string, timeoutSec int) (result *BrowserFillFormResult, err error) {
        err = browserSafeOp("BrowserFillForm", func() error {
                result, err = browserFillFormImpl(url, formData, submitSelector, timeoutSec)
                return err
        })
        return
}

func browserFillFormImpl(url string, formData FormData, submitSelector string, timeoutSec int) (*BrowserFillFormResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := getBrowserTimeout(timeoutSec)
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage()
        page = page.Context(ctx)

        // 导航到页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()
        if ctx.Err() != nil {
                return nil, fmt.Errorf("浏览器操作超时")
        }

        // 填写每个字段
        for name, value := range formData {
                selector := fmt.Sprintf("[name='%s']", name)
                element, err := page.Element(selector)
                if err != nil {
                        // 尝试 ID 选择器
                        selector = fmt.Sprintf("#%s", name)
                        element, err = page.Element(selector)
                        if err != nil {
                                log.Printf("未找到字段 '%s': %v", name, err)
                                continue
                        }
                }

                // 点击获取焦点
                element.MustClick()
                // 清空并输入
                element.SelectAllText()
                element.Input(value)
        }

        // 提交表单
        if submitSelector != "" {
                // 点击提交按钮
                btn, err := page.Element(submitSelector)
                if err != nil {
                        return nil, fmt.Errorf("未找到提交按钮: %w", err)
                }
                btn.MustClick()
        } else {
                // 按回车提交
                page.Keyboard.Press(input.Enter)
        }

        // 等待页面响应
        time.Sleep(1 * time.Second)
        page.MustWaitLoad()

        // 获取最终 URL
        info, _ := page.Info()

        return &BrowserFillFormResult{
                URL:      url,
                Success:  true,
                Message:  "表单填写并提交成功",
                FinalURL: info.URL,
        }, nil
}

// BrowserPDFResult PDF 导出结果
type BrowserPDFResult struct {
        URL     string `json:"url"`
        Success bool   `json:"success"`
        Base64  string `json:"base64,omitempty"`
}

// BrowserPDF 将页面导出为 PDF
func BrowserPDF(url string, timeoutSec int) (result *BrowserPDFResult, err error) {
        err = browserSafeOp("BrowserPDF", func() error {
                result, err = browserPDFImpl(url, timeoutSec)
                return err
        })
        return
}

func browserPDFImpl(url string, timeoutSec int) (*BrowserPDFResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := getBrowserTimeout(timeoutSec)
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage()
        page = page.Context(ctx)

        // 导航到页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()
        if ctx.Err() != nil {
                return nil, fmt.Errorf("浏览器操作超时")
        }

        // 等待页面稳定
        time.Sleep(1 * time.Second)

        // 导出 PDF
        pdf := page.MustPDF()

        return &BrowserPDFResult{
                URL:     url,
                Success: true,
                Base64:  base64.StdEncoding.EncodeToString(pdf),
        }, nil
}

// 确保 proto 包被使用（避免未导入错误）
var _ = proto.InputMouseButtonLeft

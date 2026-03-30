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

        "github.com/go-rod/rod"
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
// 浏览器会话管理辅助函数
// ============================================================

// getOrCreatePage 获取或创建浏览器页面（使用会话管理器）
func getOrCreatePage(sessionID, pageID, url string) (*rod.Page, *BrowserSession, error) {
        mgr := GetBrowserSessionManager()
        sess, ok := mgr.GetSession(sessionID)
        if !ok {
                var err error
                sess, err = mgr.CreateSession(sessionID)
                if err != nil {
                        return nil, nil, err
                }
        }

        page, ok := sess.GetPage(pageID)
        if !ok || page == nil {
                var err error
                page, err = sess.CreatePage(pageID, url)
                if err != nil {
                        return nil, nil, err
                }
        } else if url != "" {
                if err := page.Navigate(url); err != nil {
                        return nil, nil, err
                }
        }

        // 设置超时
        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()
        page = page.Context(ctx)

        return page, sess, nil
}

// ============================================================
// 交互操作类工具
// ============================================================

// BrowserClickResult 点击操作结果
type BrowserClickResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
        URL     string `json:"url,omitempty"`
}

// BrowserClick 点击页面元素（支持会话复用）
func BrowserClick(sessionID, url, selector string, timeoutSec int) (result *BrowserClickResult, err error) {
        err = browserSafeOp("BrowserClick", func() error {
                result, err = browserClickImpl(sessionID, url, selector, timeoutSec)
                return err
        })
        return
}

func browserClickImpl(sessionID, url, selector string, timeoutSec int) (*BrowserClickResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }

        // 等待元素出现
        element, err := page.Element(selector)
        if err != nil {
                return nil, fmt.Errorf("未找到元素 '%s': %w", selector, err)
        }

        if err := element.ScrollIntoView(); err != nil {
                log.Printf("滚动到元素失败: %v", err)
        }

        element.MustClick()
        time.Sleep(500 * time.Millisecond)

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

// BrowserType 在输入框中输入文本（支持会话复用）
func BrowserType(sessionID, url, selector, text string, submit bool, timeoutSec int) (result *BrowserTypeResult, err error) {
        err = browserSafeOp("BrowserType", func() error {
                result, err = browserTypeImpl(sessionID, url, selector, text, submit, timeoutSec)
                return err
        })
        return
}

func browserTypeImpl(sessionID, url, selector, text string, submit bool, timeoutSec int) (*BrowserTypeResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
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
        Height  int    `json:"height,omitempty"`
}

// BrowserScroll 滚动页面（支持会话复用）
func BrowserScroll(sessionID, url, direction string, amount int, timeoutSec int) (result *BrowserScrollResult, err error) {
        err = browserSafeOp("BrowserScroll", func() error {
                result, err = browserScrollImpl(sessionID, url, direction, amount, timeoutSec)
                return err
        })
        return
}

func browserScrollImpl(sessionID, url, direction string, amount int, timeoutSec int) (*BrowserScrollResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
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

// BrowserWaitElement 等待元素出现（支持会话复用）
func BrowserWaitElement(sessionID, url, selector string, waitTimeout int) (result *BrowserWaitResult, err error) {
        err = browserSafeOp("BrowserWaitElement", func() error {
                result, err = browserWaitElementImpl(sessionID, url, selector, waitTimeout)
                return err
        })
        return
}

func browserWaitElementImpl(sessionID, url, selector string, waitTimeout int) (*BrowserWaitResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        if waitTimeout <= 0 {
                waitTimeout = 10
        }

        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }

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

// BrowserWaitIdle 等待页面网络空闲（支持会话复用）
func BrowserWaitIdle(sessionID, url string, waitTimeout int) (result *BrowserWaitResult, err error) {
        err = browserSafeOp("BrowserWaitIdle", func() error {
                result, err = browserWaitIdleImpl(sessionID, url, waitTimeout)
                return err
        })
        return
}

func browserWaitIdleImpl(sessionID, url string, waitTimeout int) (*BrowserWaitResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        if waitTimeout <= 0 {
                waitTimeout = 10
        }

        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }

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

// BrowserExtractLinks 提取页面所有链接（支持会话复用）
func BrowserExtractLinks(sessionID, url string, timeoutSec int) (result *BrowserExtractLinksResult, err error) {
        err = browserSafeOp("BrowserExtractLinks", func() error {
                result, err = browserExtractLinksImpl(sessionID, url, timeoutSec)
                return err
        })
        return
}

func browserExtractLinksImpl(sessionID, url string, timeoutSec int) (*BrowserExtractLinksResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
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
        jsonStr = strings.Trim(jsonStr, "[]")
        if jsonStr == "" {
                return links
        }

        parts := strings.Split(jsonStr, "},{")
        for _, part := range parts {
                part = strings.Trim(part, "{}")
                link := LinkInfo{}
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

// BrowserExtractImages 提取页面所有图片（支持会话复用）
func BrowserExtractImages(sessionID, url string, timeoutSec int) (result *BrowserExtractImagesResult, err error) {
        err = browserSafeOp("BrowserExtractImages", func() error {
                result, err = browserExtractImagesImpl(sessionID, url, timeoutSec)
                return err
        })
        return
}

func browserExtractImagesImpl(sessionID, url string, timeoutSec int) (*BrowserExtractImagesResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }

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

// BrowserExtractElements 提取指定选择器的所有元素（支持会话复用）
func BrowserExtractElements(sessionID, url, selector string, includeHTML bool, timeoutSec int) (result *BrowserExtractElementsResult, err error) {
        err = browserSafeOp("BrowserExtractElements", func() error {
                result, err = browserExtractElementsImpl(sessionID, url, selector, includeHTML, timeoutSec)
                return err
        })
        return
}

func browserExtractElementsImpl(sessionID, url, selector string, includeHTML bool, timeoutSec int) (*BrowserExtractElementsResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }

        includeHTMLStr := "false"
        if includeHTML {
                includeHTMLStr = "true"
        }
        escapedSelector := strings.ReplaceAll(selector, `\`, `\\`)
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
        SavedFile string `json:"savedFile,omitempty"`
        FullPage  bool   `json:"fullPage"`
        Width     int    `json:"width"`
        Height    int    `json:"height"`
        Size      int64  `json:"size"`
}

// BrowserScreenshot 页面截图（支持会话复用）
func BrowserScreenshot(sessionID, url string, fullPage bool, timeoutSec int) (result *BrowserScreenshotResult, err error) {
        err = browserSafeOp("BrowserScreenshot", func() error {
                result, err = browserScreenshotImpl(sessionID, url, fullPage, timeoutSec)
                return err
        })
        return
}

func browserScreenshotImpl(sessionID, url string, fullPage bool, timeoutSec int) (*BrowserScreenshotResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }

        time.Sleep(1 * time.Second)

        width := page.MustEval("() => window.innerWidth").Int()
        height := page.MustEval("() => document.body.scrollHeight").Int()

        var screenshot []byte
        if fullPage {
                screenshot = page.MustScreenshotFullPage()
        } else {
                screenshot = page.MustScreenshot()
        }

        downloadDir := filepath.Join(globalExecDir, "download")
        if err := os.MkdirAll(downloadDir, 0755); err != nil {
                return nil, fmt.Errorf("创建下载目录失败: %w", err)
        }

        timestamp := time.Now().Format("20060102_150405")
        hash := md5.Sum([]byte(url))
        urlHash := fmt.Sprintf("%x", hash)[:8]
        fileName := fmt.Sprintf("screenshot_%s_%s.png", timestamp, urlHash)
        filePath := filepath.Join(downloadDir, fileName)

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

// BrowserExecuteJS 执行自定义 JavaScript（支持会话复用）
func BrowserExecuteJS(sessionID, url, script string, timeoutSec int) (result *BrowserExecuteJSResult, err error) {
        err = browserSafeOp("BrowserExecuteJS", func() error {
                result, err = browserExecuteJSImpl(sessionID, url, script, timeoutSec)
                return err
        })
        return
}

func browserExecuteJSImpl(sessionID, url, script string, timeoutSec int) (*BrowserExecuteJSResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }

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

// BrowserFillForm 填写并提交表单（支持会话复用）
func BrowserFillForm(sessionID, url string, formData FormData, submitSelector string, timeoutSec int) (result *BrowserFillFormResult, err error) {
        err = browserSafeOp("BrowserFillForm", func() error {
                result, err = browserFillFormImpl(sessionID, url, formData, submitSelector, timeoutSec)
                return err
        })
        return
}

func browserFillFormImpl(sessionID, url string, formData FormData, submitSelector string, timeoutSec int) (*BrowserFillFormResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }

        for name, value := range formData {
                selector := fmt.Sprintf("[name='%s']", name)
                element, err := page.Element(selector)
                if err != nil {
                        selector = fmt.Sprintf("#%s", name)
                        element, err = page.Element(selector)
                        if err != nil {
                                log.Printf("未找到字段 '%s': %v", name, err)
                                continue
                        }
                }
                element.MustClick()
                element.SelectAllText()
                element.Input(value)
        }

        if submitSelector != "" {
                btn, err := page.Element(submitSelector)
                if err != nil {
                        return nil, fmt.Errorf("未找到提交按钮: %w", err)
                }
                btn.MustClick()
        } else {
                page.Keyboard.Press(input.Enter)
        }

        time.Sleep(1 * time.Second)
        page.MustWaitLoad()

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

// BrowserPDF 将页面导出为 PDF（支持会话复用）
func BrowserPDF(sessionID, url string, timeoutSec int) (result *BrowserPDFResult, err error) {
        err = browserSafeOp("BrowserPDF", func() error {
                result, err = browserPDFImpl(sessionID, url, timeoutSec)
                return err
        })
        return
}

func browserPDFImpl(sessionID, url string, timeoutSec int) (*BrowserPDFResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }

        time.Sleep(1 * time.Second)
        pdf := page.MustPDF()

        return &BrowserPDFResult{
                URL:     url,
                Success: true,
                Base64:  base64.StdEncoding.EncodeToString(pdf),
        }, nil
}

// 确保 proto 包被使用
var _ = proto.InputMouseButtonLeft

// ========== 辅助类型和函数（供 browser_tools_advanced.go 使用）==========

// CookieData 用于持久化 cookie 的数据结构
type CookieData struct {
        Name     string  `json:"name"`
        Value    string  `json:"value"`
        Domain   string  `json:"domain"`
        Path     string  `json:"path"`
        Expires  float64 `json:"expires"`
        HTTPOnly bool    `json:"httpOnly"`
        Secure   bool    `json:"secure"`
        SameSite string  `json:"sameSite"`
}

// extractDomain 从 URL 提取域名（用于生成 cookie 文件名）
func extractDomain(urlStr string) string {
        urlStr = strings.TrimPrefix(urlStr, "https://")
        urlStr = strings.TrimPrefix(urlStr, "http://")
        urlStr = strings.TrimPrefix(urlStr, "www.")
        if idx := strings.Index(urlStr, "/"); idx > 0 {
                urlStr = urlStr[:idx]
        }
        urlStr = strings.ReplaceAll(urlStr, ".", "_")
        urlStr = strings.ReplaceAll(urlStr, ":", "_")
        return urlStr
}

// DevicePresets 预置设备配置（供 BrowserEmulateDevice 使用）
var DevicePresets = map[string]struct {
        Width       int
        Height      int
        UserAgent   string
        DeviceScale float64
        IsMobile    bool
        HasTouch    bool
}{
        "iphone": {
                Width:       375,
                Height:      812,
                UserAgent:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
                DeviceScale: 3,
                IsMobile:    true,
                HasTouch:    true,
        },
        "iphone_landscape": {
                Width:       812,
                Height:      375,
                UserAgent:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
                DeviceScale: 3,
                IsMobile:    true,
                HasTouch:    true,
        },
        "ipad": {
                Width:       768,
                Height:      1024,
                UserAgent:   "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
                DeviceScale: 2,
                IsMobile:    true,
                HasTouch:    true,
        },
        "android_phone": {
                Width:       360,
                Height:      800,
                UserAgent:   "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
                DeviceScale: 3,
                IsMobile:    true,
                HasTouch:    true,
        },
        "android_tablet": {
                Width:       1024,
                Height:      768,
                UserAgent:   "Mozilla/5.0 (Linux; Android 14; Pixel Tablet) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
                DeviceScale: 2,
                IsMobile:    true,
                HasTouch:    true,
        },
        "desktop": {
                Width:       1920,
                Height:      1080,
                UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
                DeviceScale: 1,
                IsMobile:    false,
                HasTouch:    false,
        },
        "desktop_mac": {
                Width:       1920,
                Height:      1080,
                UserAgent:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
                DeviceScale: 1,
                IsMobile:    false,
                HasTouch:    false,
        },
}

// BrowserPDFFromFileResult 本地文件PDF导出结果
type BrowserPDFFromFileResult struct {
        FilePath string `json:"filePath"`
        Success  bool   `json:"success"`
        Base64   string `json:"base64,omitempty"`
        Message  string `json:"message"`
}

// BrowserPDFFromFile 将本地 HTML 文件导出为 PDF（支持会话复用）
func BrowserPDFFromFile(sessionID, filePath string) (result *BrowserPDFFromFileResult, err error) {
        err = browserSafeOp("BrowserPDFFromFile", func() error {
                result, err = browserPDFFromFileImpl(sessionID, filePath)
                return err
        })
        return
}

func browserPDFFromFileImpl(sessionID, filePath string) (*BrowserPDFFromFileResult, error) {
        if _, err := os.Stat(filePath); os.IsNotExist(err) {
                return nil, fmt.Errorf("文件不存在: %s", filePath)
        }
        absPath, err := filepath.Abs(filePath)
        if err != nil {
                return nil, fmt.Errorf("获取绝对路径失败: %w", err)
        }
        fileURL := "file://" + absPath
        page, _, err := getOrCreatePage(sessionID, "default", fileURL)
        if err != nil {
                return nil, err
        }
        time.Sleep(500 * time.Millisecond)
        pdf := page.MustPDF()
        return &BrowserPDFFromFileResult{
                FilePath: filePath,
                Success:  true,
                Base64:   base64.StdEncoding.EncodeToString(pdf),
                Message:  "成功将 HTML 文件导出为 PDF",
        }, nil
}

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
        "github.com/toon-format/toon-go"
)

// ============================================================
// 高级浏览器工具增强模块
// 提供更强大的浏览器自动化能力
// ============================================================

// ============================================================
// 智能元素等待 - 等待可见、可交互、稳定
// ============================================================

// BrowserWaitForOptions 等待选项
type BrowserWaitForOptions struct {
        Visible     bool `json:"visible"`      // 等待可见
        Interactable bool `json:"interactable"` // 等待可交互
        Stable      bool `json:"stable"`       // 等待稳定
        Timeout     int  `json:"timeout"`      // 超时秒数
}

// BrowserWaitForResult 等待结果
type BrowserWaitForResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
        Found   bool   `json:"found"`
}

// BrowserWaitForSmart 智能等待元素
func BrowserWaitForSmart(url, selector string, opts BrowserWaitForOptions) (result *BrowserWaitForResult, err error) {
        err = browserSafeOp("BrowserWaitForSmart", func() error {
                result, err = browserWaitForSmartImpl(url, selector, opts)
                return err
        })
        return
}

func browserWaitForSmartImpl(url, selector string, opts BrowserWaitForOptions) (*BrowserWaitForResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        if opts.Timeout <= 0 {
                opts.Timeout = 10
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        // 等待元素出现
        element, err := page.Element(selector)
        if err != nil {
                return &BrowserWaitForResult{
                        Success: false,
                        Found:   false,
                        Message: fmt.Sprintf("未找到元素 '%s'", selector),
                }, nil
        }

        // 可选：等待可见
        if opts.Visible {
                if err := element.WaitVisible(); err != nil {
                        return &BrowserWaitForResult{
                                Success: false,
                                Found:   true,
                                Message: fmt.Sprintf("元素 '%s' 等待可见超时", selector),
                        }, nil
                }
        }

        // 可选：等待可交互
        if opts.Interactable {
                if _, err := element.WaitInteractable(); err != nil {
                        return &BrowserWaitForResult{
                                Success: false,
                                Found:   true,
                                Message: fmt.Sprintf("元素 '%s' 等待可交互超时", selector),
                        }, nil
                }
        }

        // 可选：等待稳定
        if opts.Stable {
                if err := element.WaitStable(time.Duration(opts.Timeout) * time.Second); err != nil {
                        return &BrowserWaitForResult{
                                Success: false,
                                Found:   true,
                                Message: fmt.Sprintf("元素 '%s' 等待稳定超时", selector),
                        }, nil
                }
        }

        return &BrowserWaitForResult{
                Success: true,
                Found:   true,
                Message: fmt.Sprintf("元素 '%s' 已准备好", selector),
        }, nil
}

// ============================================================
// 高级交互操作 - 拖拽、悬停、双击、右键
// ============================================================

// BrowserHoverResult 悬停结果
type BrowserHoverResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
}

// BrowserHover 鼠标悬停在元素上
func BrowserHover(url, selector string) (result *BrowserHoverResult, err error) {
        err = browserSafeOp("BrowserHover", func() error {
                result, err = browserHoverImpl(url, selector)
                return err
        })
        return
}

func browserHoverImpl(url, selector string) (*BrowserHoverResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        element, err := page.Element(selector)
        if err != nil {
                return nil, fmt.Errorf("未找到元素 '%s': %w", selector, err)
        }

        // 使用 Rod 的 Hover 方法（会自动滚动到元素并等待可交互）
        if err := element.Hover(); err != nil {
                return nil, fmt.Errorf("悬停失败: %w", err)
        }

        return &BrowserHoverResult{
                Success: true,
                Message: fmt.Sprintf("成功悬停在元素: %s", selector),
        }, nil
}

// BrowserDoubleClickResult 双击结果
type BrowserDoubleClickResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
        URL     string `json:"url,omitempty"`
}

// BrowserDoubleClick 双击元素
func BrowserDoubleClick(url, selector string) (result *BrowserDoubleClickResult, err error) {
        err = browserSafeOp("BrowserDoubleClick", func() error {
                result, err = browserDoubleClickImpl(url, selector)
                return err
        })
        return
}

func browserDoubleClickImpl(url, selector string) (*BrowserDoubleClickResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        element, err := page.Element(selector)
        if err != nil {
                return nil, fmt.Errorf("未找到元素 '%s': %w", selector, err)
        }

        // 双击 (clickCount = 2)
        if err := element.Click(proto.InputMouseButtonLeft, 2); err != nil {
                return nil, fmt.Errorf("双击失败: %w", err)
        }

        time.Sleep(500 * time.Millisecond)

        info, _ := page.Info()

        return &BrowserDoubleClickResult{
                Success: true,
                Message: fmt.Sprintf("成功双击元素: %s", selector),
                URL:     info.URL,
        }, nil
}

// BrowserRightClickResult 右键点击结果
type BrowserRightClickResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
}

// BrowserRightClick 右键点击元素
func BrowserRightClick(url, selector string) (result *BrowserRightClickResult, err error) {
        err = browserSafeOp("BrowserRightClick", func() error {
                result, err = browserRightClickImpl(url, selector)
                return err
        })
        return
}

func browserRightClickImpl(url, selector string) (*BrowserRightClickResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        element, err := page.Element(selector)
        if err != nil {
                return nil, fmt.Errorf("未找到元素 '%s': %w", selector, err)
        }

        // 右键点击
        if err := element.Click(proto.InputMouseButtonRight, 1); err != nil {
                return nil, fmt.Errorf("右键点击失败: %w", err)
        }

        return &BrowserRightClickResult{
                Success: true,
                Message: fmt.Sprintf("成功右键点击元素: %s", selector),
        }, nil
}

// BrowserDragResult 拖拽结果
type BrowserDragResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
}

// BrowserDrag 拖拽元素到目标位置
func BrowserDrag(url, sourceSelector, targetSelector string) (result *BrowserDragResult, err error) {
        err = browserSafeOp("BrowserDrag", func() error {
                result, err = browserDragImpl(url, sourceSelector, targetSelector)
                return err
        })
        return
}

func browserDragImpl(url, sourceSelector, targetSelector string) (*BrowserDragResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        // 获取源元素
        sourceElement, err := page.Element(sourceSelector)
        if err != nil {
                return nil, fmt.Errorf("未找到源元素 '%s': %w", sourceSelector, err)
        }

        // 获取目标元素
        targetElement, err := page.Element(targetSelector)
        if err != nil {
                return nil, fmt.Errorf("未找到目标元素 '%s': %w", targetSelector, err)
        }

        // 获取源元素的位置
        sourceShape, err := sourceElement.Shape()
        if err != nil {
                return nil, fmt.Errorf("获取源元素形状失败: %w", err)
        }
        sourceBox := sourceShape.Box()

        // 获取目标元素的位置
        targetShape, err := targetElement.Shape()
        if err != nil {
                return nil, fmt.Errorf("获取目标元素形状失败: %w", err)
        }
        targetBox := targetShape.Box()

        // 执行拖拽
        // 1. 移动到源元素中心
        sourceX := sourceBox.X + sourceBox.Width/2
        sourceY := sourceBox.Y + sourceBox.Height/2
        if err := page.Mouse.MoveTo(proto.Point{X: sourceX, Y: sourceY}); err != nil {
                return nil, fmt.Errorf("移动到源元素失败: %w", err)
        }

        // 2. 按下鼠标左键
        if err := page.Mouse.Down(proto.InputMouseButtonLeft, 1); err != nil {
                return nil, fmt.Errorf("按下鼠标失败: %w", err)
        }

        // 3. 移动到目标元素中心（模拟人类拖拽，分多步移动）
        targetX := targetBox.X + targetBox.Width/2
        targetY := targetBox.Y + targetBox.Height/2
        if err := page.Mouse.MoveLinear(proto.Point{X: targetX, Y: targetY}, 10); err != nil {
                return nil, fmt.Errorf("拖拽移动失败: %w", err)
        }

        // 4. 释放鼠标左键
        if err := page.Mouse.Up(proto.InputMouseButtonLeft, 1); err != nil {
                return nil, fmt.Errorf("释放鼠标失败: %w", err)
        }

        return &BrowserDragResult{
                Success: true,
                Message: fmt.Sprintf("成功将元素 '%s' 拖拽到 '%s'", sourceSelector, targetSelector),
        }, nil
}

// ============================================================
// 导航操作 - 前进/后退/刷新
// ============================================================

// BrowserNavigateResult 导航结果
type BrowserNavigateResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
        URL     string `json:"url"`
        Title   string `json:"title"`
}

// BrowserNavigateBack 后退
func BrowserNavigateBack(url string) (result *BrowserNavigateResult, err error) {
        err = browserSafeOp("BrowserNavigateBack", func() error {
                result, err = browserNavigateBackImpl(url)
                return err
        })
        return
}

func browserNavigateBackImpl(url string) (*BrowserNavigateResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        // 后退
        if err := page.NavigateBack(); err != nil {
                return nil, fmt.Errorf("后退失败: %w", err)
        }

        page.MustWaitLoad()

        info, _ := page.Info()

        return &BrowserNavigateResult{
                Success: true,
                Message: "成功后退",
                URL:     info.URL,
                Title:   info.Title,
        }, nil
}

// BrowserNavigateForward 前进
func BrowserNavigateForward(url string) (result *BrowserNavigateResult, err error) {
        err = browserSafeOp("BrowserNavigateForward", func() error {
                result, err = browserNavigateForwardImpl(url)
                return err
        })
        return
}

func browserNavigateForwardImpl(url string) (*BrowserNavigateResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        // 前进
        if err := page.NavigateForward(); err != nil {
                return nil, fmt.Errorf("前进失败: %w", err)
        }

        page.MustWaitLoad()

        info, _ := page.Info()

        return &BrowserNavigateResult{
                Success: true,
                Message: "成功前进",
                URL:     info.URL,
                Title:   info.Title,
        }, nil
}

// BrowserRefresh 刷新页面
func BrowserRefresh(url string) (result *BrowserNavigateResult, err error) {
        err = browserSafeOp("BrowserRefresh", func() error {
                result, err = browserRefreshImpl(url)
                return err
        })
        return
}

func browserRefreshImpl(url string) (*BrowserNavigateResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        // 刷新
        if err := page.Reload(); err != nil {
                return nil, fmt.Errorf("刷新失败: %w", err)
        }

        info, _ := page.Info()

        return &BrowserNavigateResult{
                Success: true,
                Message: "成功刷新页面",
                URL:     info.URL,
                Title:   info.Title,
        }, nil
}

// ============================================================
// Cookie 管理
// ============================================================

// CookieInfo Cookie 信息
type CookieInfo struct {
        Name   string `json:"name"`
        Value  string `json:"value"`
        Domain string `json:"domain"`
        Path   string `json:"path"`
}

// BrowserGetCookiesResult 获取 Cookies 结果
type BrowserGetCookiesResult struct {
        URL     string       `json:"url"`
        Count   int          `json:"count"`
        Cookies []CookieInfo `json:"cookies"`
}

// BrowserGetCookies 获取页面 Cookies
func BrowserGetCookies(url string) (result *BrowserGetCookiesResult, err error) {
        err = browserSafeOp("BrowserGetCookies", func() error {
                result, err = browserGetCookiesImpl(url)
                return err
        })
        return
}

func browserGetCookiesImpl(url string) (*BrowserGetCookiesResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        cookies, err := page.Cookies([]string{url})
        if err != nil {
                return nil, fmt.Errorf("获取 Cookies 失败: %w", err)
        }

        var cookieInfos []CookieInfo
        for _, c := range cookies {
                cookieInfos = append(cookieInfos, CookieInfo{
                        Name:   c.Name,
                        Value:  c.Value,
                        Domain: c.Domain,
                        Path:   c.Path,
                })
        }

        return &BrowserGetCookiesResult{
                URL:     url,
                Count:   len(cookieInfos),
                Cookies: cookieInfos,
        }, nil
}

// BrowserSetCookiesResult 设置 Cookies 结果
type BrowserSetCookiesResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
}

// BrowserSetCookies 设置页面 Cookies
func BrowserSetCookies(url string, cookies []CookieInfo) (result *BrowserSetCookiesResult, err error) {
        err = browserSafeOp("BrowserSetCookies", func() error {
                result, err = browserSetCookiesImpl(url, cookies)
                return err
        })
        return
}

func browserSetCookiesImpl(url string, cookies []CookieInfo) (*BrowserSetCookiesResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        // 转换为 proto 格式
        var protoCookies []*proto.NetworkCookieParam
        for _, c := range cookies {
                protoCookies = append(protoCookies, &proto.NetworkCookieParam{
                        Name:   c.Name,
                        Value:  c.Value,
                        Domain: c.Domain,
                        Path:   c.Path,
                })
        }

        if err := page.SetCookies(protoCookies); err != nil {
                return nil, fmt.Errorf("设置 Cookies 失败: %w", err)
        }

        return &BrowserSetCookiesResult{
                Success: true,
                Message: fmt.Sprintf("成功设置 %d 个 Cookies", len(cookies)),
        }, nil
}

// ============================================================
// Cookie 文件持久化
// ============================================================

// BrowserCookieSaveResult 保存Cookie结果
type BrowserCookieSaveResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
        Count   int    `json:"count"`
        File    string `json:"file"`
}

// BrowserCookieSave 保存页面 Cookies 到文件
// url: 要获取Cookie的页面URL
// filePath: 保存Cookie的文件路径（如不指定则使用默认路径）
func BrowserCookieSave(url, filePath string) (result *BrowserCookieSaveResult, err error) {
        err = browserSafeOp("BrowserCookieSave", func() error {
                result, err = browserCookieSaveImpl(url, filePath)
                return err
        })
        return
}

func browserCookieSaveImpl(url, filePath string) (*BrowserCookieSaveResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        // 默认保存路径
        if filePath == "" {
                // 从URL提取域名作为文件名
                domain := extractDomain(url)
                filePath = fmt.Sprintf("cookies_%s.toon", domain)
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        // 获取所有 Cookies
        cookies, err := page.Cookies([]string{})
        if err != nil {
                return nil, fmt.Errorf("获取 Cookies 失败: %w", err)
        }

        // 转换为可序列化格式
        var cookieData []CookieData
        for _, c := range cookies {
                cookieData = append(cookieData, CookieData{
                        Name:     c.Name,
                        Value:    c.Value,
                        Domain:   c.Domain,
                        Path:     c.Path,
                        Expires:  float64(c.Expires),
                        HTTPOnly: c.HTTPOnly,
                        Secure:   c.Secure,
                        SameSite: string(c.SameSite),
                })
        }

        // 序列化为 TOON 格式
        toonData, err := toon.Marshal(cookieData)
        if err != nil {
                return nil, fmt.Errorf("序列化 Cookies 失败: %w", err)
        }

        // 写入文件
        if err := os.WriteFile(filePath, toonData, 0644); err != nil {
                return nil, fmt.Errorf("写入文件失败: %w", err)
        }

        return &BrowserCookieSaveResult{
                Success: true,
                Message: fmt.Sprintf("成功保存 %d 个 Cookies", len(cookieData)),
                Count:   len(cookieData),
                File:    filePath,
        }, nil
}

// BrowserCookieLoadResult 加载Cookie结果
type BrowserCookieLoadResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
        Count   int    `json:"count"`
        File    string `json:"file"`
        URL     string `json:"url"`
}

// BrowserCookieLoad 从文件加载 Cookies 并应用到页面
// url: 要应用Cookie的目标页面URL
// filePath: Cookie文件路径
func BrowserCookieLoad(url, filePath string) (result *BrowserCookieLoadResult, err error) {
        err = browserSafeOp("BrowserCookieLoad", func() error {
                result, err = browserCookieLoadImpl(url, filePath)
                return err
        })
        return
}

func browserCookieLoadImpl(url, filePath string) (*BrowserCookieLoadResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        // 检查文件是否存在
        if _, err := os.Stat(filePath); os.IsNotExist(err) {
                return nil, fmt.Errorf("Cookie 文件不存在: %s", filePath)
        }

        // 读取文件
        toonData, err := os.ReadFile(filePath)
        if err != nil {
                return nil, fmt.Errorf("读取文件失败: %w", err)
        }

        // 解析 TOON 格式
        var cookieData []CookieData
        if err := toon.Unmarshal(toonData, &cookieData); err != nil {
                return nil, fmt.Errorf("解析 Cookie 文件失败: %w", err)
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        // 先设置 Cookies，再导航
        var protoCookies []*proto.NetworkCookieParam
        for _, c := range cookieData {
                protoCookies = append(protoCookies, &proto.NetworkCookieParam{
                        Name:     c.Name,
                        Value:    c.Value,
                        Domain:   c.Domain,
                        Path:     c.Path,
                        Expires:  proto.TimeSinceEpoch(c.Expires),
                        HTTPOnly: c.HTTPOnly,
                        Secure:   c.Secure,
                        SameSite: proto.NetworkCookieSameSite(c.SameSite),
                })
        }

        if len(protoCookies) > 0 {
                if err := page.SetCookies(protoCookies); err != nil {
                        log.Printf("设置 Cookies 警告: %v", err)
                }
        }

        // 导航到目标页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        info, _ := page.Info()

        return &BrowserCookieLoadResult{
                Success: true,
                Message: fmt.Sprintf("成功加载 %d 个 Cookies 并应用到页面", len(cookieData)),
                Count:   len(cookieData),
                File:    filePath,
                URL:     info.URL,
        }, nil
}

// CookieData Cookie 数据结构（用于文件持久化）
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

// extractDomain 从URL提取域名
func extractDomain(urlStr string) string {
        // 简单提取域名
        urlStr = strings.TrimPrefix(urlStr, "https://")
        urlStr = strings.TrimPrefix(urlStr, "http://")
        urlStr = strings.TrimPrefix(urlStr, "www.")
        
        // 取第一个 / 之前的部分
        if idx := strings.Index(urlStr, "/"); idx > 0 {
                urlStr = urlStr[:idx]
        }
        
        // 替换特殊字符
        urlStr = strings.ReplaceAll(urlStr, ".", "_")
        urlStr = strings.ReplaceAll(urlStr, ":", "_")
        
        return urlStr
}

// ============================================================
// 对话框处理 - alert/confirm/prompt
// ============================================================

// BrowserHandleDialogResult 对话框处理结果
type BrowserHandleDialogResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
}

// BrowserHandleDialog 处理对话框
// accept: true 接受，false 取消
// promptText: prompt 对话框的输入文本
func BrowserHandleDialog(url, triggerSelector string, accept bool, promptText string) (result *BrowserHandleDialogResult, err error) {
        err = browserSafeOp("BrowserHandleDialog", func() error {
                result, err = browserHandleDialogImpl(url, triggerSelector, accept, promptText)
                return err
        })
        return
}

func browserHandleDialogImpl(url, triggerSelector string, accept bool, promptText string) (*BrowserHandleDialogResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        // 设置对话框处理器
        wait, handle := page.HandleDialog()

        // 在另一个 goroutine 中触发对话框
        go func() {
                if triggerSelector != "" {
                        element, err := page.Element(triggerSelector)
                        if err == nil {
                                element.MustClick()
                        }
                }
        }()

        // 等待对话框
        dialog := wait()

        // 处理对话框
        err = handle(&proto.PageHandleJavaScriptDialog{
                Accept:     accept,
                PromptText: promptText,
        })
        if err != nil {
                return nil, fmt.Errorf("处理对话框失败: %w", err)
        }

        action := "取消"
        if accept {
                action = "接受"
        }

        return &BrowserHandleDialogResult{
                Success: true,
                Message: fmt.Sprintf("成功%s对话框: %s", action, dialog.Message),
        }, nil
}

// ============================================================
// 文件上传
// ============================================================

// BrowserUploadFileResult 文件上传结果
type BrowserUploadFileResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
}

// BrowserUploadFile 上传文件
func BrowserUploadFile(url, fileInputSelector string, filePaths []string) (result *BrowserUploadFileResult, err error) {
        err = browserSafeOp("BrowserUploadFile", func() error {
                result, err = browserUploadFileImpl(url, fileInputSelector, filePaths)
                return err
        })
        return
}

func browserUploadFileImpl(url, fileInputSelector string, filePaths []string) (*BrowserUploadFileResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        element, err := page.Element(fileInputSelector)
        if err != nil {
                return nil, fmt.Errorf("未找到文件输入框 '%s': %w", fileInputSelector, err)
        }

        // 设置文件
        if err := element.SetFiles(filePaths); err != nil {
                return nil, fmt.Errorf("上传文件失败: %w", err)
        }

        return &BrowserUploadFileResult{
                Success: true,
                Message: fmt.Sprintf("成功上传 %d 个文件", len(filePaths)),
        }, nil
}

// ============================================================
// 页面快照 - 简化DOM用于视觉分析
// ============================================================

// PageSnapshotElement 快照元素
type PageSnapshotElement struct {
        Tag      string                  `json:"tag"`
        Text     string                  `json:"text,omitempty"`
        ID       string                  `json:"id,omitempty"`
        Class    string                  `json:"class,omitempty"`
        Href     string                  `json:"href,omitempty"`
        Src      string                  `json:"src,omitempty"`
        Children []PageSnapshotElement   `json:"children,omitempty"`
        Attrs    map[string]string       `json:"attrs,omitempty"`
        Rect     *ElementRect            `json:"rect,omitempty"`
}

// ElementRect 元素位置
type ElementRect struct {
        X      float64 `json:"x"`
        Y      float64 `json:"y"`
        Width  float64 `json:"width"`
        Height float64 `json:"height"`
}

// BrowserSnapshotResult 页面快照结果
type BrowserSnapshotResult struct {
        URL       string                `json:"url"`
        Title     string                `json:"title"`
        Snapshot  *PageSnapshotElement  `json:"snapshot,omitempty"`  // 小数据直接返回
        SavedFile string                `json:"savedFile,omitempty"` // 大数据保存到文件
        Length    int                   `json:"length"`              // 原始数据长度
}

// BrowserSnapshot 获取页面快照（简化DOM）
func BrowserSnapshot(url string, maxDepth int) (result *BrowserSnapshotResult, err error) {
        err = browserSafeOp("BrowserSnapshot", func() error {
                result, err = browserSnapshotImpl(url, maxDepth)
                return err
        })
        return
}

func browserSnapshotImpl(url string, maxDepth int) (*BrowserSnapshotResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        if maxDepth <= 0 {
                maxDepth = 5
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        // 获取页面标题
        info, _ := page.Info()

        // 执行快照脚本
        snapshotJSON := page.MustEval(`() => {
                function getSnapshot(el, depth, maxDepth) {
                        if (depth > maxDepth) return null;
                        
                        const result = {
                                tag: el.tagName ? el.tagName.toLowerCase() : '',
                                text: '',
                                attrs: {}
                        };
                        
                        // 获取属性
                        if (el.attributes) {
                                for (let attr of el.attributes) {
                                        result.attrs[attr.name] = attr.value;
                                }
                        }
                        
                        // 常用属性单独提取
                        result.id = el.id || '';
                        result.class = el.className || '';
                        result.href = el.href || '';
                        result.src = el.src || '';
                        
                        // 获取位置
                        const rect = el.getBoundingClientRect();
                        if (rect) {
                                result.rect = {
                                        x: rect.x,
                                        y: rect.y,
                                        width: rect.width,
                                        height: rect.height
                                };
                        }
                        
                        // 处理子元素
                        const children = [];
                        let textContent = '';
                        
                        for (let child of el.childNodes) {
                                if (child.nodeType === Node.TEXT_NODE) {
                                        textContent += child.textContent.trim() + ' ';
                                } else if (child.nodeType === Node.ELEMENT_NODE) {
                                        const childSnapshot = getSnapshot(child, depth + 1, maxDepth);
                                        if (childSnapshot) {
                                                children.push(childSnapshot);
                                        }
                                }
                        }
                        
                        result.text = textContent.trim().substring(0, 200);
                        if (children.length > 0) {
                                result.children = children;
                        }
                        
                        return result;
                }
                
                return JSON.stringify(getSnapshot(document.body, 0, ` + fmt.Sprintf("%d", maxDepth) + `));
        }`).Str()

        var snapshot PageSnapshotElement
        if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
                log.Printf("解析快照失败: %v", err)
        }

        // 检查数据大小，过大的保存到文件
        maxDirectLen := 16000 // 最大直接返回字符数
        snapshotLen := len(snapshotJSON)

        if snapshotLen > maxDirectLen {
                // 保存到文件（TOON 格式）（基于程序所在目录）
                downloadDir := filepath.Join(globalExecDir, "download")
                if err := os.MkdirAll(downloadDir, 0755); err != nil {
                        // 降级：返回截断的快照
                        log.Printf("创建下载目录失败: %v", err)
                        return &BrowserSnapshotResult{
                                URL:      url,
                                Title:    info.Title,
                                Snapshot: &snapshot,
                                Length:   snapshotLen,
                        }, nil
                }

                timestamp := time.Now().Format("20060102_150405")
                hash := md5.Sum([]byte(url))
                urlHash := fmt.Sprintf("%x", hash)[:8]
                fileName := fmt.Sprintf("snapshot_%s_%s.toon", timestamp, urlHash)
                filePath := filepath.Join(downloadDir, fileName)

                // 使用 TOON 格式序列化
                snapshotTOON, err := toon.Marshal(&snapshot)
                if err != nil {
                        log.Printf("TOON 序列化失败: %v", err)
                        snapshotTOON = []byte(snapshotJSON) // 降级使用 JSON
                }

                // 写入文件
                contentToSave := fmt.Sprintf("URL: %s\nTitle: %s\nMaxDepth: %d\nDate: %s\n\n%s",
                        url, info.Title, maxDepth, time.Now().Format("2006-01-02 15:04:05"), string(snapshotTOON))
                if err := os.WriteFile(filePath, []byte(contentToSave), 0644); err != nil {
                        log.Printf("保存快照失败: %v", err)
                } else {
                        fmt.Println("Snapshot saved to: " + filePath)
                        return &BrowserSnapshotResult{
                                URL:       url,
                                Title:     info.Title,
                                SavedFile: filePath,
                                Length:    snapshotLen,
                        }, nil
                }
        }

        return &BrowserSnapshotResult{
                URL:      url,
                Title:    info.Title,
                Snapshot: &snapshot,
                Length:   snapshotLen,
        }, nil
}

// ============================================================
// iframe 操作
// ============================================================

// BrowserIframeResult iframe 操作结果
type BrowserIframeResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
}

// BrowserIframeEnter 进入 iframe
func BrowserIframeEnter(url, iframeSelector string) (result *BrowserIframeResult, err error) {
        err = browserSafeOp("BrowserIframeEnter", func() error {
                result, err = browserIframeEnterImpl(url, iframeSelector)
                return err
        })
        return
}

func browserIframeEnterImpl(url, iframeSelector string) (*BrowserIframeResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        element, err := page.Element(iframeSelector)
        if err != nil {
                return nil, fmt.Errorf("未找到 iframe '%s': %w", iframeSelector, err)
        }

        // 进入 iframe
        frame, err := element.Frame()
        if err != nil {
                return nil, fmt.Errorf("进入 iframe 失败: %w", err)
        }

        // 等待 iframe 加载
        if err := frame.WaitLoad(); err != nil {
                log.Printf("iframe 加载警告: %v", err)
        }

        frameInfo, _ := frame.Info()

        return &BrowserIframeResult{
                Success: true,
                Message: fmt.Sprintf("成功进入 iframe，当前 URL: %s", frameInfo.URL),
        }, nil
}

// ============================================================
// 增强的键盘操作
// ============================================================

// BrowserKeyPressResult 按键结果
type BrowserKeyPressResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
}

// BrowserKeyPress 模拟按键组合
func BrowserKeyPress(url string, keys []string) (result *BrowserKeyPressResult, err error) {
        err = browserSafeOp("BrowserKeyPress", func() error {
                result, err = browserKeyPressImpl(url, keys)
                return err
        })
        return
}

func browserKeyPressImpl(url string, keys []string) (*BrowserKeyPressResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        // 创建按键动作
        actions := page.KeyActions()
        for _, key := range keys {
                // 转换按键名称
                var k input.Key
                switch strings.ToLower(key) {
                case "enter":
                        k = input.Enter
                case "tab":
                        k = input.Tab
                case "escape", "esc":
                        k = input.Escape
                case "backspace":
                        k = input.Backspace
                case "delete":
                        k = input.Delete
                case "arrowup", "up":
                        k = input.ArrowUp
                case "arrowdown", "down":
                        k = input.ArrowDown
                case "arrowleft", "left":
                        k = input.ArrowLeft
                case "arrowright", "right":
                        k = input.ArrowRight
                case "control", "ctrl":
                        k = input.ControlLeft
                case "alt":
                        k = input.AltLeft
                case "shift":
                        k = input.ShiftLeft
                case "meta", "cmd", "command":
                        k = input.MetaLeft
                default:
                        // 单字符
                        if len(key) == 1 {
                                k = input.Key(key[0])
                        } else {
                                continue
                        }
                }
                actions = actions.Press(k)
        }

        if err := actions.Do(); err != nil {
                return nil, fmt.Errorf("按键失败: %w", err)
        }

        return &BrowserKeyPressResult{
                Success: true,
                Message: fmt.Sprintf("成功按下键: %s", strings.Join(keys, "+")),
        }, nil
}

// ============================================================
// 选择下拉框
// ============================================================

// BrowserSelectResult 选择结果
type BrowserSelectResult struct {
        Success bool     `json:"success"`
        Message string   `json:"message"`
        Values  []string `json:"values"`
}

// BrowserSelectOption 选择下拉框选项
func BrowserSelectOption(url, selector string, values []string) (result *BrowserSelectResult, err error) {
        err = browserSafeOp("BrowserSelectOption", func() error {
                result, err = browserSelectOptionImpl(url, selector, values)
                return err
        })
        return
}

func browserSelectOptionImpl(url, selector string, values []string) (*BrowserSelectResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        element, err := page.Element(selector)
        if err != nil {
                return nil, fmt.Errorf("未找到下拉框 '%s': %w", selector, err)
        }

        // 选择选项
        if err := element.Select(values, true, rod.SelectorTypeText); err != nil {
                return nil, fmt.Errorf("选择选项失败: %w", err)
        }

        return &BrowserSelectResult{
                Success: true,
                Message: fmt.Sprintf("成功选择选项: %v", values),
                Values:  values,
        }, nil
}

// ============================================================
// 元素截图
// ============================================================

// BrowserElementScreenshotResult 元素截图结果
type BrowserElementScreenshotResult struct {
        Success bool   `json:"success"`
        Base64  string `json:"base64"`
}

// BrowserElementScreenshot 元素截图
func BrowserElementScreenshot(url, selector string) (result *BrowserElementScreenshotResult, err error) {
        err = browserSafeOp("BrowserElementScreenshot", func() error {
                result, err = browserElementScreenshotImpl(url, selector)
                return err
        })
        return
}

func browserElementScreenshotImpl(url, selector string) (*BrowserElementScreenshotResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        element, err := page.Element(selector)
        if err != nil {
                return nil, fmt.Errorf("未找到元素 '%s': %w", selector, err)
        }

        // 元素截图
        screenshot, err := element.Screenshot(proto.PageCaptureScreenshotFormatPng, 100)
        if err != nil {
                return nil, fmt.Errorf("截图失败: %w", err)
        }

        return &BrowserElementScreenshotResult{
                Success: true,
                Base64:  base64.StdEncoding.EncodeToString(screenshot),
        }, nil
}

// ============================================================
// 批量操作
// ============================================================

// BrowserBatchClickResult 批量点击结果
type BrowserBatchClickResult struct {
        Success int      `json:"success"`
        Failed  int      `json:"failed"`
        Total   int      `json:"total"`
        Results []string `json:"results"`
}

// BrowserBatchClick 批量点击多个元素
func BrowserBatchClick(url string, selectors []string) (result *BrowserBatchClickResult, err error) {
        err = browserSafeOp("BrowserBatchClick", func() error {
                result, err = browserBatchClickImpl(url, selectors)
                return err
        })
        return
}

func browserBatchClickImpl(url string, selectors []string) (*BrowserBatchClickResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        result := &BrowserBatchClickResult{
                Total:   len(selectors),
                Results: []string{},
        }

        for _, selector := range selectors {
                element, err := page.Element(selector)
                if err != nil {
                        result.Failed++
                        result.Results = append(result.Results, fmt.Sprintf("未找到: %s", selector))
                        continue
                }

                if err := element.Click(proto.InputMouseButtonLeft, 1); err != nil {
                        result.Failed++
                        result.Results = append(result.Results, fmt.Sprintf("点击失败: %s", selector))
                        continue
                }

                result.Success++
                result.Results = append(result.Results, fmt.Sprintf("成功: %s", selector))
                time.Sleep(300 * time.Millisecond)
        }

        return result, nil
}

// ============================================================
// 自定义请求头和 User-Agent 设置
// ============================================================

// BrowserSetHeadersResult 设置请求头结果
type BrowserSetHeadersResult struct {
        Success bool     `json:"success"`
        Message string   `json:"message"`
        Headers []string `json:"headers"`
}

// BrowserSetHeaders 设置自定义请求头并访问页面
// headers: 格式为 "Key: Value" 的字符串数组
func BrowserSetHeaders(url string, headers []string) (result *BrowserSetHeadersResult, err error) {
        err = browserSafeOp("BrowserSetHeaders", func() error {
                result, err = browserSetHeadersImpl(url, headers)
                return err
        })
        return
}

func browserSetHeadersImpl(url string, headers []string) (*BrowserSetHeadersResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        // 解析并设置请求头
        var headerPairs []string
        for _, h := range headers {
                parts := strings.SplitN(h, ":", 2)
                if len(parts) == 2 {
                        key := strings.TrimSpace(parts[0])
                        value := strings.TrimSpace(parts[1])
                        headerPairs = append(headerPairs, key, value)
                }
        }

        if len(headerPairs) > 0 {
                // 使用 SetExtraHeaders 设置请求头
                page.SetExtraHeaders(headerPairs)
        }

        // 导航到页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        return &BrowserSetHeadersResult{
                Success: true,
                Message: fmt.Sprintf("成功设置 %d 个请求头并访问页面", len(headerPairs)/2),
                Headers: headers,
        }, nil
}

// BrowserSetUserAgentResult 设置 UserAgent 结果
type BrowserSetUserAgentResult struct {
        Success    bool   `json:"success"`
        Message    string `json:"message"`
        UserAgent  string `json:"userAgent"`
        URL        string `json:"url"`
}

// BrowserSetUserAgent 设置自定义 User-Agent 并访问页面
func BrowserSetUserAgent(url, userAgent string) (result *BrowserSetUserAgentResult, err error) {
        err = browserSafeOp("BrowserSetUserAgent", func() error {
                result, err = browserSetUserAgentImpl(url, userAgent)
                return err
        })
        return
}

func browserSetUserAgentImpl(url, userAgent string) (*BrowserSetUserAgentResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        if userAgent == "" {
                return nil, fmt.Errorf("User-Agent 不能为空")
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        // 设置 User-Agent
        page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
                UserAgent: userAgent,
        })

        // 导航到页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        info, _ := page.Info()

        return &BrowserSetUserAgentResult{
                Success:   true,
                Message:   fmt.Sprintf("成功设置 User-Agent 并访问页面"),
                UserAgent: userAgent,
                URL:       info.URL,
        }, nil
}

// ============================================================
// 本地文件 PDF 导出
// ============================================================

// BrowserPDFFromFileResult 本地文件PDF导出结果
type BrowserPDFFromFileResult struct {
        FilePath string `json:"filePath"`
        Success  bool   `json:"success"`
        Base64   string `json:"base64,omitempty"`
        Message  string `json:"message"`
}

// BrowserPDFFromFile 将本地 HTML 文件导出为 PDF
// filePath: 本地 HTML 文件的绝对路径
func BrowserPDFFromFile(filePath string) (result *BrowserPDFFromFileResult, err error) {
        err = browserSafeOp("BrowserPDFFromFile", func() error {
                result, err = browserPDFFromFileImpl(filePath)
                return err
        })
        return
}

func browserPDFFromFileImpl(filePath string) (*BrowserPDFFromFileResult, error) {
        // 检查文件是否存在
        if _, err := os.Stat(filePath); os.IsNotExist(err) {
                return nil, fmt.Errorf("文件不存在: %s", filePath)
        }

        // 转换为 file:// URL
        absPath, err := filepath.Abs(filePath)
        if err != nil {
                return nil, fmt.Errorf("获取绝对路径失败: %w", err)
        }
        fileURL := "file://" + absPath

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        // 导航到本地文件
        if err := page.Navigate(fileURL); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        // 等待页面稳定
        time.Sleep(500 * time.Millisecond)

        // 导出 PDF
        pdf := page.MustPDF()

        return &BrowserPDFFromFileResult{
                FilePath: filePath,
                Success:  true,
                Base64:   base64.StdEncoding.EncodeToString(pdf),
                Message:  "成功将 HTML 文件导出为 PDF",
        }, nil
}

// ============================================================
// 设备模拟
// ============================================================

// DevicePreset 预置设备配置
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

// BrowserEmulateDeviceResult 设备模拟结果
type BrowserEmulateDeviceResult struct {
        Success    bool   `json:"success"`
        Message    string `json:"message"`
        Device     string `json:"device"`
        Width      int    `json:"width"`
        Height     int    `json:"height"`
        UserAgent  string `json:"userAgent"`
        URL        string `json:"url"`
}

// BrowserEmulateDevice 模拟设备访问页面
// device: 预置设备名称，可选值: iphone, iphone_landscape, ipad, android_phone, android_tablet, desktop, desktop_mac
func BrowserEmulateDevice(url, device string) (result *BrowserEmulateDeviceResult, err error) {
        err = browserSafeOp("BrowserEmulateDevice", func() error {
                result, err = browserEmulateDeviceImpl(url, device)
                return err
        })
        return
}

func browserEmulateDeviceImpl(url, device string) (*BrowserEmulateDeviceResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }

        // 获取设备配置
        preset, ok := DevicePresets[device]
        if !ok {
                return nil, fmt.Errorf("未知的设备预设: %s，可用值: iphone, iphone_landscape, ipad, android_phone, android_tablet, desktop, desktop_mac", device)
        }

        timeout := globalTimeoutConfig.Browser
        if timeout <= 0 {
                timeout = DefaultBrowserTimeout
        }
        ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
        defer cancel()

        browser, err := launchBrowserRod()
        if err != nil {
                return nil, err
        }
        defer browser.Close()

        page := browser.MustPage().Context(ctx)

        // 设置视口和设备模拟
        page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
                Width:             preset.Width,
                Height:            preset.Height,
                DeviceScaleFactor: preset.DeviceScale,
                Mobile:            preset.IsMobile,
        })

        // 设置 User-Agent
        page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
                UserAgent: preset.UserAgent,
        })

        // 导航到页面
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }

        page.MustWaitLoad()

        info, _ := page.Info()

        return &BrowserEmulateDeviceResult{
                Success:   true,
                Message:   fmt.Sprintf("成功模拟 %s 设备访问页面", device),
                Device:    device,
                Width:     preset.Width,
                Height:    preset.Height,
                UserAgent: preset.UserAgent,
                URL:       info.URL,
        }, nil
}



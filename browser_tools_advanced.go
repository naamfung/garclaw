package main

import (
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
// 高级浏览器工具（已集成会话管理）
// 注意：本文件中的函数都接受 sessionID 作为第一个参数
// ============================================================

// ---------- 悬停 ----------
type BrowserHoverResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
}

func BrowserHover(sessionID, url, selector string) (result *BrowserHoverResult, err error) {
        err = browserSafeOp("BrowserHover", func() error {
                result, err = browserHoverImpl(sessionID, url, selector)
                return err
        })
        return
}

func browserHoverImpl(sessionID, url, selector string) (*BrowserHoverResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
        element, err := page.Element(selector)
        if err != nil {
                return nil, fmt.Errorf("未找到元素 '%s': %w", selector, err)
        }
        if err := element.Hover(); err != nil {
                return nil, fmt.Errorf("悬停失败: %w", err)
        }
        return &BrowserHoverResult{
                Success: true,
                Message: fmt.Sprintf("成功悬停在元素: %s", selector),
        }, nil
}

// ---------- 双击 ----------
type BrowserDoubleClickResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
        URL     string `json:"url,omitempty"`
}

func BrowserDoubleClick(sessionID, url, selector string) (result *BrowserDoubleClickResult, err error) {
        err = browserSafeOp("BrowserDoubleClick", func() error {
                result, err = browserDoubleClickImpl(sessionID, url, selector)
                return err
        })
        return
}

func browserDoubleClickImpl(sessionID, url, selector string) (*BrowserDoubleClickResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
        element, err := page.Element(selector)
        if err != nil {
                return nil, fmt.Errorf("未找到元素 '%s': %w", selector, err)
        }
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

// ---------- 右键 ----------
type BrowserRightClickResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
}

func BrowserRightClick(sessionID, url, selector string) (result *BrowserRightClickResult, err error) {
        err = browserSafeOp("BrowserRightClick", func() error {
                result, err = browserRightClickImpl(sessionID, url, selector)
                return err
        })
        return
}

func browserRightClickImpl(sessionID, url, selector string) (*BrowserRightClickResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
        element, err := page.Element(selector)
        if err != nil {
                return nil, fmt.Errorf("未找到元素 '%s': %w", selector, err)
        }
        if err := element.Click(proto.InputMouseButtonRight, 1); err != nil {
                return nil, fmt.Errorf("右键点击失败: %w", err)
        }
        return &BrowserRightClickResult{
                Success: true,
                Message: fmt.Sprintf("成功右键点击元素: %s", selector),
        }, nil
}

// ---------- 拖拽 ----------
type BrowserDragResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
}

func BrowserDrag(sessionID, url, sourceSelector, targetSelector string) (result *BrowserDragResult, err error) {
        err = browserSafeOp("BrowserDrag", func() error {
                result, err = browserDragImpl(sessionID, url, sourceSelector, targetSelector)
                return err
        })
        return
}

func browserDragImpl(sessionID, url, sourceSelector, targetSelector string) (*BrowserDragResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
        sourceElement, err := page.Element(sourceSelector)
        if err != nil {
                return nil, fmt.Errorf("未找到源元素 '%s': %w", sourceSelector, err)
        }
        targetElement, err := page.Element(targetSelector)
        if err != nil {
                return nil, fmt.Errorf("未找到目标元素 '%s': %w", targetSelector, err)
        }
        sourceShape, err := sourceElement.Shape()
        if err != nil {
                return nil, err
        }
        sourceBox := sourceShape.Box()
        targetShape, err := targetElement.Shape()
        if err != nil {
                return nil, err
        }
        targetBox := targetShape.Box()
        sourceX := sourceBox.X + sourceBox.Width/2
        sourceY := sourceBox.Y + sourceBox.Height/2
        if err := page.Mouse.MoveTo(proto.Point{X: sourceX, Y: sourceY}); err != nil {
                return nil, err
        }
        if err := page.Mouse.Down(proto.InputMouseButtonLeft, 1); err != nil {
                return nil, err
        }
        targetX := targetBox.X + targetBox.Width/2
        targetY := targetBox.Y + targetBox.Height/2
        if err := page.Mouse.MoveLinear(proto.Point{X: targetX, Y: targetY}, 10); err != nil {
                return nil, err
        }
        if err := page.Mouse.Up(proto.InputMouseButtonLeft, 1); err != nil {
                return nil, err
        }
        return &BrowserDragResult{
                Success: true,
                Message: fmt.Sprintf("成功将元素 '%s' 拖拽到 '%s'", sourceSelector, targetSelector),
        }, nil
}

// ---------- 智能等待 ----------
type BrowserWaitForOptions struct {
        Visible      bool
        Interactable bool
        Stable       bool
        Timeout      int
}

func BrowserWaitForSmart(sessionID, url, selector string, opts BrowserWaitForOptions) (result *BrowserWaitResult, err error) {
        err = browserSafeOp("BrowserWaitForSmart", func() error {
                result, err = browserWaitForSmartImpl(sessionID, url, selector, opts)
                return err
        })
        return
}

func browserWaitForSmartImpl(sessionID, url, selector string, opts BrowserWaitForOptions) (*BrowserWaitResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        if opts.Timeout <= 0 {
                opts.Timeout = 10
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
        element, err := page.Element(selector)
        if err != nil {
                return &BrowserWaitResult{
                        Success: false,
                        Message: fmt.Sprintf("未找到元素 '%s'", selector),
                }, nil
        }
        if opts.Visible {
                if err := element.WaitVisible(); err != nil {
                        return &BrowserWaitResult{
                                Success: false,
                                Message: fmt.Sprintf("元素 '%s' 等待可见超时", selector),
                        }, nil
                }
        }
        if opts.Interactable {
                if _, err := element.WaitInteractable(); err != nil {
                        return &BrowserWaitResult{
                                Success: false,
                                Message: fmt.Sprintf("元素 '%s' 等待可交互超时", selector),
                        }, nil
                }
        }
        if opts.Stable {
                if err := element.WaitStable(time.Duration(opts.Timeout) * time.Second); err != nil {
                        return &BrowserWaitResult{
                                Success: false,
                                Message: fmt.Sprintf("元素 '%s' 等待稳定超时", selector),
                        }, nil
                }
        }
        return &BrowserWaitResult{
                Success: true,
                Message: fmt.Sprintf("元素 '%s' 已准备好", selector),
        }, nil
}

// ---------- 导航 ----------
type BrowserNavigateResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
        URL     string `json:"url"`
        Title   string `json:"title"`
}

func BrowserNavigateBack(sessionID, url string) (result *BrowserNavigateResult, err error) {
        err = browserSafeOp("BrowserNavigateBack", func() error {
                result, err = browserNavigateBackImpl(sessionID, url)
                return err
        })
        return
}

func browserNavigateBackImpl(sessionID, url string) (*BrowserNavigateResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
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

func BrowserNavigateForward(sessionID, url string) (result *BrowserNavigateResult, err error) {
        err = browserSafeOp("BrowserNavigateForward", func() error {
                result, err = browserNavigateForwardImpl(sessionID, url)
                return err
        })
        return
}

func browserNavigateForwardImpl(sessionID, url string) (*BrowserNavigateResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
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

func BrowserRefresh(sessionID, url string) (result *BrowserNavigateResult, err error) {
        err = browserSafeOp("BrowserRefresh", func() error {
                result, err = browserRefreshImpl(sessionID, url)
                return err
        })
        return
}

func browserRefreshImpl(sessionID, url string) (*BrowserNavigateResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
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

// ---------- Cookie 管理 ----------
type BrowserGetCookiesResult struct {
        URL     string       `json:"url"`
        Count   int          `json:"count"`
        Cookies []CookieInfo `json:"cookies"`
}

type CookieInfo struct {
        Name   string `json:"name"`
        Value  string `json:"value"`
        Domain string `json:"domain"`
        Path   string `json:"path"`
}

func BrowserGetCookies(sessionID, url string) (result *BrowserGetCookiesResult, err error) {
        err = browserSafeOp("BrowserGetCookies", func() error {
                result, err = browserGetCookiesImpl(sessionID, url)
                return err
        })
        return
}

func browserGetCookiesImpl(sessionID, url string) (*BrowserGetCookiesResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
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

type BrowserCookieSaveResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
        Count   int    `json:"count"`
        File    string `json:"file"`
}

func BrowserCookieSave(sessionID, url, filePath string) (result *BrowserCookieSaveResult, err error) {
        err = browserSafeOp("BrowserCookieSave", func() error {
                result, err = browserCookieSaveImpl(sessionID, url, filePath)
                return err
        })
        return
}

func browserCookieSaveImpl(sessionID, url, filePath string) (*BrowserCookieSaveResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        if filePath == "" {
                domain := extractDomain(url)
                filePath = fmt.Sprintf("cookies_%s.toon", domain)
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
        cookies, err := page.Cookies([]string{})
        if err != nil {
                return nil, fmt.Errorf("获取 Cookies 失败: %w", err)
        }
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
        toonData, err := toon.Marshal(cookieData)
        if err != nil {
                return nil, fmt.Errorf("序列化 Cookies 失败: %w", err)
        }
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

type BrowserCookieLoadResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
        Count   int    `json:"count"`
        File    string `json:"file"`
        URL     string `json:"url"`
}

func BrowserCookieLoad(sessionID, url, filePath string) (result *BrowserCookieLoadResult, err error) {
        err = browserSafeOp("BrowserCookieLoad", func() error {
                result, err = browserCookieLoadImpl(sessionID, url, filePath)
                return err
        })
        return
}

func browserCookieLoadImpl(sessionID, url, filePath string) (*BrowserCookieLoadResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        if _, err := os.Stat(filePath); os.IsNotExist(err) {
                return nil, fmt.Errorf("Cookie 文件不存在: %s", filePath)
        }
        toonData, err := os.ReadFile(filePath)
        if err != nil {
                return nil, fmt.Errorf("读取文件失败: %w", err)
        }
        var cookieData []CookieData
        if err := toon.Unmarshal(toonData, &cookieData); err != nil {
                return nil, fmt.Errorf("解析 Cookie 文件失败: %w", err)
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
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

// ---------- 页面快照 ----------
type BrowserSnapshotResult struct {
        URL       string               `json:"url"`
        Title     string               `json:"title"`
        Snapshot  *PageSnapshotElement `json:"snapshot,omitempty"`
        SavedFile string               `json:"savedFile,omitempty"`
        Length    int                  `json:"length"`
}

type PageSnapshotElement struct {
        Tag      string                `json:"tag"`
        Text     string                `json:"text,omitempty"`
        ID       string                `json:"id,omitempty"`
        Class    string                `json:"class,omitempty"`
        Href     string                `json:"href,omitempty"`
        Src      string                `json:"src,omitempty"`
        Children []PageSnapshotElement `json:"children,omitempty"`
        Attrs    map[string]string     `json:"attrs,omitempty"`
        Rect     *ElementRect          `json:"rect,omitempty"`
}

type ElementRect struct {
        X      float64 `json:"x"`
        Y      float64 `json:"y"`
        Width  float64 `json:"width"`
        Height float64 `json:"height"`
}

func BrowserSnapshot(sessionID, url string, maxDepth int) (result *BrowserSnapshotResult, err error) {
        err = browserSafeOp("BrowserSnapshot", func() error {
                result, err = browserSnapshotImpl(sessionID, url, maxDepth)
                return err
        })
        return
}

func browserSnapshotImpl(sessionID, url string, maxDepth int) (*BrowserSnapshotResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        if maxDepth <= 0 {
                maxDepth = 5
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
        info, _ := page.Info()
        snapshotJSON := page.MustEval(`() => {
                function getSnapshot(el, depth, maxDepth) {
                        if (depth > maxDepth) return null;
                        const result = { tag: el.tagName ? el.tagName.toLowerCase() : '', text: '', attrs: {} };
                        if (el.attributes) {
                                for (let attr of el.attributes) result.attrs[attr.name] = attr.value;
                        }
                        result.id = el.id || '';
                        result.class = el.className || '';
                        result.href = el.href || '';
                        result.src = el.src || '';
                        const rect = el.getBoundingClientRect();
                        if (rect) result.rect = { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
                        let textContent = '';
                        const children = [];
                        for (let child of el.childNodes) {
                                if (child.nodeType === Node.TEXT_NODE) {
                                        textContent += child.textContent.trim() + ' ';
                                } else if (child.nodeType === Node.ELEMENT_NODE) {
                                        const childSnapshot = getSnapshot(child, depth + 1, maxDepth);
                                        if (childSnapshot) children.push(childSnapshot);
                                }
                        }
                        result.text = textContent.trim().substring(0, 200);
                        if (children.length > 0) result.children = children;
                        return result;
                }
                return JSON.stringify(getSnapshot(document.body, 0, ` + fmt.Sprintf("%d", maxDepth) + `));
        }`).Str()

        var snapshot PageSnapshotElement
        if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
                log.Printf("解析快照失败: %v", err)
        }

        maxDirectLen := 16000
        snapshotLen := len(snapshotJSON)
        if snapshotLen > maxDirectLen {
                downloadDir := filepath.Join(globalExecDir, "download")
                os.MkdirAll(downloadDir, 0755)
                timestamp := time.Now().Format("20060102_150405")
                hash := md5.Sum([]byte(url))
                urlHash := fmt.Sprintf("%x", hash)[:8]
                fileName := fmt.Sprintf("snapshot_%s_%s.toon", timestamp, urlHash)
                filePath := filepath.Join(downloadDir, fileName)
                snapshotTOON, _ := toon.Marshal(&snapshot)
                contentToSave := fmt.Sprintf("URL: %s\nTitle: %s\nMaxDepth: %d\nDate: %s\n\n%s",
                        url, info.Title, maxDepth, time.Now().Format("2006-01-02 15:04:05"), string(snapshotTOON))
                os.WriteFile(filePath, []byte(contentToSave), 0644)
                return &BrowserSnapshotResult{
                        URL:       url,
                        Title:     info.Title,
                        SavedFile: filePath,
                        Length:    snapshotLen,
                }, nil
        }
        return &BrowserSnapshotResult{
                URL:      url,
                Title:    info.Title,
                Snapshot: &snapshot,
                Length:   snapshotLen,
        }, nil
}

// ---------- 文件上传 ----------
type BrowserUploadFileResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
}

func BrowserUploadFile(sessionID, url, fileInputSelector string, filePaths []string) (result *BrowserUploadFileResult, err error) {
        err = browserSafeOp("BrowserUploadFile", func() error {
                result, err = browserUploadFileImpl(sessionID, url, fileInputSelector, filePaths)
                return err
        })
        return
}

func browserUploadFileImpl(sessionID, url, fileInputSelector string, filePaths []string) (*BrowserUploadFileResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
        element, err := page.Element(fileInputSelector)
        if err != nil {
                return nil, fmt.Errorf("未找到文件输入框 '%s': %w", fileInputSelector, err)
        }
        if err := element.SetFiles(filePaths); err != nil {
                return nil, fmt.Errorf("上传文件失败: %w", err)
        }
        return &BrowserUploadFileResult{
                Success: true,
                Message: fmt.Sprintf("成功上传 %d 个文件", len(filePaths)),
        }, nil
}

// ---------- 下拉选择 ----------
type BrowserSelectOptionResult struct {
        Success bool     `json:"success"`
        Message string   `json:"message"`
        Values  []string `json:"values"`
}

func BrowserSelectOption(sessionID, url, selector string, values []string) (result *BrowserSelectOptionResult, err error) {
        err = browserSafeOp("BrowserSelectOption", func() error {
                result, err = browserSelectOptionImpl(sessionID, url, selector, values)
                return err
        })
        return
}

func browserSelectOptionImpl(sessionID, url, selector string, values []string) (*BrowserSelectOptionResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
        element, err := page.Element(selector)
        if err != nil {
                return nil, fmt.Errorf("未找到下拉框 '%s': %w", selector, err)
        }
        if err := element.Select(values, true, rod.SelectorTypeText); err != nil {
                return nil, fmt.Errorf("选择选项失败: %w", err)
        }
        return &BrowserSelectOptionResult{
                Success: true,
                Message: fmt.Sprintf("成功选择选项: %v", values),
                Values:  values,
        }, nil
}

// ---------- 键盘按键 ----------
type BrowserKeyPressResult struct {
        Success bool   `json:"success"`
        Message string `json:"message"`
}

func BrowserKeyPress(sessionID, url string, keys []string) (result *BrowserKeyPressResult, err error) {
        err = browserSafeOp("BrowserKeyPress", func() error {
                result, err = browserKeyPressImpl(sessionID, url, keys)
                return err
        })
        return
}

func browserKeyPressImpl(sessionID, url string, keys []string) (*BrowserKeyPressResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
        actions := page.KeyActions()
        for _, key := range keys {
                var k input.Key
                switch strings.ToLower(key) {
                case "enter": k = input.Enter
                case "tab": k = input.Tab
                case "escape", "esc": k = input.Escape
                case "backspace": k = input.Backspace
                case "delete": k = input.Delete
                case "arrowup", "up": k = input.ArrowUp
                case "arrowdown", "down": k = input.ArrowDown
                case "arrowleft", "left": k = input.ArrowLeft
                case "arrowright", "right": k = input.ArrowRight
                case "control", "ctrl": k = input.ControlLeft
                case "alt": k = input.AltLeft
                case "shift": k = input.ShiftLeft
                case "meta", "cmd", "command": k = input.MetaLeft
                default:
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

// ---------- 元素截图 ----------
type BrowserElementScreenshotResult struct {
        Success bool   `json:"success"`
        Base64  string `json:"base64"`
}

func BrowserElementScreenshot(sessionID, url, selector string) (result *BrowserElementScreenshotResult, err error) {
        err = browserSafeOp("BrowserElementScreenshot", func() error {
                result, err = browserElementScreenshotImpl(sessionID, url, selector)
                return err
        })
        return
}

func browserElementScreenshotImpl(sessionID, url, selector string) (*BrowserElementScreenshotResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
        element, err := page.Element(selector)
        if err != nil {
                return nil, fmt.Errorf("未找到元素 '%s': %w", selector, err)
        }
        screenshot, err := element.Screenshot(proto.PageCaptureScreenshotFormatPng, 100)
        if err != nil {
                return nil, fmt.Errorf("截图失败: %w", err)
        }
        return &BrowserElementScreenshotResult{
                Success: true,
                Base64:  base64.StdEncoding.EncodeToString(screenshot),
        }, nil
}

// ---------- 设备模拟 ----------
type BrowserEmulateDeviceResult struct {
        Success   bool   `json:"success"`
        Message   string `json:"message"`
        Device    string `json:"device"`
        Width     int    `json:"width"`
        Height    int    `json:"height"`
        UserAgent string `json:"userAgent"`
        URL       string `json:"url"`
}

func BrowserEmulateDevice(sessionID, url, device string) (result *BrowserEmulateDeviceResult, err error) {
        err = browserSafeOp("BrowserEmulateDevice", func() error {
                result, err = browserEmulateDeviceImpl(sessionID, url, device)
                return err
        })
        return
}

func browserEmulateDeviceImpl(sessionID, url, device string) (*BrowserEmulateDeviceResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        preset, ok := DevicePresets[device]
        if !ok {
                return nil, fmt.Errorf("未知的设备预设: %s", device)
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
        page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
                Width:             preset.Width,
                Height:            preset.Height,
                DeviceScaleFactor: preset.DeviceScale,
                Mobile:            preset.IsMobile,
        })
        page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: preset.UserAgent})
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

// ---------- 设置请求头 ----------
type BrowserSetHeadersResult struct {
        Success bool     `json:"success"`
        Message string   `json:"message"`
        Headers []string `json:"headers"`
}

func BrowserSetHeaders(sessionID, url string, headers []string) (result *BrowserSetHeadersResult, err error) {
        err = browserSafeOp("BrowserSetHeaders", func() error {
                result, err = browserSetHeadersImpl(sessionID, url, headers)
                return err
        })
        return
}

func browserSetHeadersImpl(sessionID, url string, headers []string) (*BrowserSetHeadersResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        var headerPairs []string
        for _, h := range headers {
                parts := strings.SplitN(h, ":", 2)
                if len(parts) == 2 {
                        headerPairs = append(headerPairs, strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
                }
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
        if len(headerPairs) > 0 {
                page.SetExtraHeaders(headerPairs)
        }
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

// ---------- 设置 User-Agent ----------
type BrowserSetUserAgentResult struct {
        Success   bool   `json:"success"`
        Message   string `json:"message"`
        UserAgent string `json:"userAgent"`
        URL       string `json:"url"`
}

func BrowserSetUserAgent(sessionID, url, userAgent string) (result *BrowserSetUserAgentResult, err error) {
        err = browserSafeOp("BrowserSetUserAgent", func() error {
                result, err = browserSetUserAgentImpl(sessionID, url, userAgent)
                return err
        })
        return
}

func browserSetUserAgentImpl(sessionID, url, userAgent string) (*BrowserSetUserAgentResult, error) {
        if err := ValidateURLForFetch(url); err != nil {
                return nil, err
        }
        if userAgent == "" {
                return nil, fmt.Errorf("User-Agent 不能为空")
        }
        page, _, err := getOrCreatePage(sessionID, "default", url)
        if err != nil {
                return nil, err
        }
        page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: userAgent})
        if err := page.Navigate(url); err != nil {
                return nil, fmt.Errorf("导航失败: %w", err)
        }
        page.MustWaitLoad()
        info, _ := page.Info()
        return &BrowserSetUserAgentResult{
                Success:   true,
                Message:   "成功设置 User-Agent 并访问页面",
                UserAgent: userAgent,
                URL:       info.URL,
        }, nil
}

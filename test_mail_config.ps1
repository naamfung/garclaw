# 测试1: 默认配置（IsMailHog=false）
Write-Host "=== Test 1: Default configuration (IsMailHog=false) ==="
Remove-Item Env:IS_MAILHOG -ErrorAction SilentlyContinue
.arclaw.exe "-c" "/mail ser@gar.local Test Email This is a test email from GarClaw"

# 测试2: IsMailHog=true
Write-Host "\n=== Test 2: IsMailHog=true ==="
$env:IS_MAILHOG="true"
.arclaw.exe "-c" "/mail ser@gar.local Test Email This is a test email from GarClaw"

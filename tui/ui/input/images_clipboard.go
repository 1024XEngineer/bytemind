package tui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"bytemind/internal/llm"
)

type clipboardImageReader interface {
	ReadImage(ctx context.Context) (mediaType string, data []byte, fileName string, err error)
}

type defaultClipboardImageReader struct{}

func (defaultClipboardImageReader) ReadImage(ctx context.Context) (string, []byte, string, error) {
	if runtime.GOOS != "windows" {
		return "", nil, "", llm.WrapError("clipboard", llm.ErrorCodeClipboardUnavailable, fmt.Errorf("clipboard image is only supported on windows in this build"))
	}

	script := strings.TrimSpace(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

function Get-MediaTypeFromExtension([string]$path) {
	if ([string]::IsNullOrWhiteSpace($path)) { return '' }
	$ext = [System.IO.Path]::GetExtension($path).ToLowerInvariant()
	if ($ext -eq '.png') { return 'image/png' }
	elseif ($ext -eq '.jpg' -or $ext -eq '.jpeg') { return 'image/jpeg' }
	elseif ($ext -eq '.webp') { return 'image/webp' }
	elseif ($ext -eq '.gif') { return 'image/gif' }
	return ''
}

function Build-Payload([string]$mediaType, [string]$fileName, [byte[]]$bytes) {
	if ([string]::IsNullOrWhiteSpace($mediaType)) { return '' }
	if ($null -eq $bytes -or $bytes.Length -le 0) { return '' }
	if ([string]::IsNullOrWhiteSpace($fileName)) { $fileName = 'clipboard.png' }
	return (@{media_type=$mediaType;file_name=$fileName;data=[Convert]::ToBase64String($bytes)} | ConvertTo-Json -Compress)
}

function Payload-FromPath([string]$path) {
	if ([string]::IsNullOrWhiteSpace($path)) { return '' }
	$candidate = $path.Trim().Trim('"')
	if (-not [System.IO.File]::Exists($candidate)) { return '' }
	$mediaType = Get-MediaTypeFromExtension $candidate
	if ($mediaType -ne '') {
		$bytes = [System.IO.File]::ReadAllBytes($candidate)
		return (Build-Payload $mediaType ([System.IO.Path]::GetFileName($candidate)) $bytes)
	}
	try {
		$img = [System.Drawing.Image]::FromFile($candidate)
		try {
			$ms = New-Object System.IO.MemoryStream
			$img.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
			$name = [System.IO.Path]::GetFileNameWithoutExtension($candidate) + '.png'
			return (Build-Payload 'image/png' $name $ms.ToArray())
		} finally {
			$img.Dispose()
		}
	} catch {
		return ''
	}
}

function Payload-FromDataUri([string]$value) {
	if ([string]::IsNullOrWhiteSpace($value)) { return '' }
	$trimmed = $value.Trim()
	if ($trimmed -notmatch '^data:(?<media>image/[A-Za-z0-9.+-]+);base64,(?<data>.+)$') { return '' }
	$mediaType = $Matches['media'].ToLowerInvariant()
	try {
		$bytes = [Convert]::FromBase64String($Matches['data'])
	} catch {
		return ''
	}
	$fileName = 'clipboard.png'
	if ($mediaType -eq 'image/jpeg') { $fileName = 'clipboard.jpg' }
	elseif ($mediaType -eq 'image/webp') { $fileName = 'clipboard.webp' }
	elseif ($mediaType -eq 'image/gif') { $fileName = 'clipboard.gif' }
	return (Build-Payload $mediaType $fileName $bytes)
}

function Payload-FromImageUrl([string]$value) {
	if ([string]::IsNullOrWhiteSpace($value)) { return '' }
	$urlText = $value.Trim()
	if ($urlText -like 'file://*') {
		try {
			$uri = [System.Uri]$urlText
			return (Payload-FromPath $uri.LocalPath)
		} catch {
			return ''
		}
	}
	if ($urlText -notmatch '^https?://') { return '' }
	try {
		$uri = [System.Uri]$urlText
	} catch {
		return ''
	}
	$mediaType = Get-MediaTypeFromExtension $uri.AbsolutePath
	if ($mediaType -eq '') { return '' }
	try {
		$wc = New-Object System.Net.WebClient
		$bytes = $wc.DownloadData($uri)
		$fileName = [System.IO.Path]::GetFileName($uri.AbsolutePath)
		if ([string]::IsNullOrWhiteSpace($fileName)) {
			$fileName = 'clipboard' + [System.IO.Path]::GetExtension($uri.AbsolutePath)
		}
		return (Build-Payload $mediaType $fileName $bytes)
	} catch {
		return ''
	}
}

function Payload-FromHtml([string]$html) {
	if ([string]::IsNullOrWhiteSpace($html)) { return '' }
	$pattern = '(?is)<img[^>]+src\s*=\s*["''](?<src>[^"''>]+)["'']'
	$match = [System.Text.RegularExpressions.Regex]::Match($html, $pattern)
	if (-not $match.Success) { return '' }
	$src = [System.Net.WebUtility]::HtmlDecode($match.Groups['src'].Value)
	$payload = Payload-FromDataUri $src
	if (-not [string]::IsNullOrWhiteSpace($payload)) { return $payload }
	$payload = Payload-FromPath $src
	if (-not [string]::IsNullOrWhiteSpace($payload)) { return $payload }
	$payload = Payload-FromImageUrl $src
	if (-not [string]::IsNullOrWhiteSpace($payload)) { return $payload }
	return ''
}

$dataObj = [System.Windows.Forms.Clipboard]::GetDataObject()
if ($null -eq $dataObj) { return '' }
$payload = ''

if ($dataObj.GetDataPresent('PNG')) {
	$png = $dataObj.GetData('PNG')
	$pngBytes = $null
	if ($png -is [System.IO.MemoryStream]) { $pngBytes = $png.ToArray() }
	elseif ($png -is [byte[]]) { $pngBytes = $png }
	elseif ($png -is [System.IO.Stream]) {
		$ms = New-Object System.IO.MemoryStream
		$png.CopyTo($ms)
		$pngBytes = $ms.ToArray()
	}
	if ($null -ne $pngBytes -and $pngBytes.Length -gt 0) {
		$payload = Build-Payload 'image/png' 'clipboard.png' $pngBytes
		if (-not [string]::IsNullOrWhiteSpace($payload)) { $payload; return }
	}
}

if ($dataObj.GetDataPresent([System.Windows.Forms.DataFormats]::FileDrop)) {
	$files = $dataObj.GetData([System.Windows.Forms.DataFormats]::FileDrop)
	$fileList = @()
	if ($files -is [System.Collections.Specialized.StringCollection]) { $fileList = @($files | ForEach-Object { $_ }) }
	elseif ($files -is [string[]]) { $fileList = $files }
	elseif ($files -is [string]) { $fileList = @($files) }
	foreach ($path in $fileList) {
		$payload = Payload-FromPath $path
		if (-not [string]::IsNullOrWhiteSpace($payload)) { $payload; return }
	}
}

if ([System.Windows.Forms.Clipboard]::ContainsImage()) {
	$img = [System.Windows.Forms.Clipboard]::GetImage()
	if ($null -ne $img) {
		try {
			$ms = New-Object System.IO.MemoryStream
			$img.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
			$payload = Build-Payload 'image/png' 'clipboard.png' $ms.ToArray()
			if (-not [string]::IsNullOrWhiteSpace($payload)) { $payload; return }
		} finally {
			$img.Dispose()
		}
	}
}

if ($dataObj.GetDataPresent([System.Windows.Forms.DataFormats]::Text)) {
	$text = [System.Windows.Forms.Clipboard]::GetText()
	$payload = Payload-FromDataUri $text
	if (-not [string]::IsNullOrWhiteSpace($payload)) { $payload; return }
	$payload = Payload-FromPath $text
	if (-not [string]::IsNullOrWhiteSpace($payload)) { $payload; return }
	$payload = Payload-FromImageUrl $text
	if (-not [string]::IsNullOrWhiteSpace($payload)) { $payload; return }
}

if ($dataObj.GetDataPresent([System.Windows.Forms.DataFormats]::Html)) {
	$html = $dataObj.GetData([System.Windows.Forms.DataFormats]::Html).ToString()
	$payload = Payload-FromHtml $html
	if (-not [string]::IsNullOrWhiteSpace($payload)) { $payload; return }
}

return ''
`)

	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command", script).CombinedOutput()
	if err != nil {
		return "", nil, "", llm.WrapError("clipboard", llm.ErrorCodeClipboardUnavailable, fmt.Errorf("clipboard image read failed: %s", strings.TrimSpace(string(out))))
	}
	mediaType, data, fileName, parseErr := parseClipboardImageOutput(string(out))
	if parseErr != nil {
		if strings.Contains(strings.ToLower(parseErr.Error()), "no image") {
			return "", nil, "", llm.WrapError("clipboard", llm.ErrorCodeClipboardUnavailable, parseErr)
		}
		return "", nil, "", llm.WrapError("clipboard", llm.ErrorCodeImageDecodeFailed, parseErr)
	}
	if len(data) == 0 {
		return "", nil, "", llm.WrapError("clipboard", llm.ErrorCodeImageDecodeFailed, fmt.Errorf("clipboard image is empty"))
	}
	if strings.TrimSpace(mediaType) == "" {
		mediaType = "image/png"
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = "clipboard.png"
	}
	return mediaType, data, fileName, nil
}

func parseClipboardImageOutput(raw string) (string, []byte, string, error) {
	encoded := normalizeClipboardOutput(raw)
	if encoded == "" {
		return "", nil, "", llm.WrapError("clipboard", llm.ErrorCodeClipboardUnavailable, fmt.Errorf("clipboard has no image"))
	}

	type payload struct {
		MediaType string `json:"media_type"`
		FileName  string `json:"file_name"`
		Data      string `json:"data"`
	}
	if strings.HasPrefix(encoded, "{") {
		var p payload
		if err := json.Unmarshal([]byte(encoded), &p); err == nil && strings.TrimSpace(p.Data) != "" {
			decoded, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(p.Data))
			if decodeErr != nil {
				return "", nil, "", decodeErr
			}
			return strings.TrimSpace(p.MediaType), decoded, strings.TrimSpace(p.FileName), nil
		}
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", nil, "", err
	}
	return "image/png", data, "clipboard.png", nil
}

func normalizeClipboardOutput(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

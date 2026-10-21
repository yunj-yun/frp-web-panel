/**
 * 复制文本到剪贴板，返回是否成功。任何异常都会被吞掉并返回 false，调用方无需 try/catch。
 *
 * 为什么需要降级：`navigator.clipboard` 只在安全上下文（HTTPS 或 localhost）中可用。
 * 本项目通常通过 `http://<局域网IP>:9000` 访问，属于非安全上下文，此时
 * `navigator.clipboard` 为 undefined，直接调用会抛 TypeError。因此不可用或写入失败时，
 * 降级到 `document.execCommand('copy')`，保证内网 HTTP 环境下的复制按钮依然可用。
 */
export async function copyText(text: string): Promise<boolean> {
  try {
    if (
      typeof navigator !== 'undefined' &&
      typeof window !== 'undefined' &&
      window.isSecureContext &&
      typeof navigator.clipboard?.writeText === 'function'
    ) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // 安全上下文下也可能被浏览器策略拒绝，继续尝试降级方案
  }
  try {
    return execCommandCopy(text)
  } catch {
    return false
  }
}

/**
 * 降级方案：用隐藏的 textarea + document.execCommand('copy')。
 * 元素使用 position: fixed 避免改变布局，结束后恢复滚动位置与焦点。
 */
function execCommandCopy(text: string): boolean {
  let textarea: HTMLTextAreaElement | null = null
  let activeElement: HTMLElement | null = null
  let scrollX = 0
  let scrollY = 0

  try {
    if (typeof document === 'undefined' || typeof document.body === 'undefined') {
      return false
    }

    activeElement = document.activeElement as HTMLElement | null
    scrollX = window.scrollX
    scrollY = window.scrollY
    textarea = document.createElement('textarea')
    textarea.value = text
    textarea.setAttribute('readonly', '')
    textarea.style.position = 'fixed'
    textarea.style.top = '-9999px'
    textarea.style.left = '-9999px'
    textarea.style.opacity = '0'
    textarea.style.pointerEvents = 'none'
    document.body.appendChild(textarea)
    textarea.focus({ preventScroll: true })
    textarea.select()
    textarea.setSelectionRange(0, textarea.value.length)
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    // 清理本身也不能抛异常，否则会破坏"绝不抛出"的约定
    try {
      if (textarea && textarea.parentNode) {
        textarea.parentNode.removeChild(textarea)
      }
      if (activeElement && typeof activeElement.focus === 'function') {
        activeElement.focus({ preventScroll: true })
      }
      window.scrollTo(scrollX, scrollY)
    } catch {
      // 恢复焦点/滚动失败不影响复制结果
    }
  }
}

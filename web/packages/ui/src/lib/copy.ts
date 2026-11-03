// Copies text, using a selection fallback where the async clipboard is unavailable (plain HTTP).
export async function copyText(text: string): Promise<boolean> {
  if (
    typeof navigator !== 'undefined' &&
    typeof window !== 'undefined' &&
    window.isSecureContext &&
    navigator.clipboard?.writeText
  ) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      /* Try the selection-based method below. */
    }
  }
  if (typeof document === 'undefined' || !document.body) return false;
  const active = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  const field = active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement ? active : null;
  const fieldSelection = field
    ? { start: field.selectionStart, end: field.selectionEnd, direction: field.selectionDirection }
    : null;
  const selection = window.getSelection();
  const ranges = selection
    ? Array.from({ length: selection.rangeCount }, (_, index) => selection.getRangeAt(index).cloneRange())
    : [];
  const temporary = document.createElement('textarea');
  temporary.value = text;
  temporary.readOnly = true;
  temporary.setAttribute('aria-hidden', 'true');
  temporary.style.cssText = 'position:fixed;left:-9999px;top:0;opacity:0';
  document.body.appendChild(temporary);
  try {
    temporary.focus({ preventScroll: true });
    temporary.select();
    temporary.setSelectionRange(0, temporary.value.length);
    return document.execCommand('copy');
  } catch {
    return false;
  } finally {
    temporary.remove();
    if (selection) {
      selection.removeAllRanges();
      for (const range of ranges) selection.addRange(range);
    }
    if (active) {
      try {
        active.focus({ preventScroll: true });
      } catch {
        active.focus();
      }
      if (
        field &&
        fieldSelection?.start !== null &&
        fieldSelection?.start !== undefined &&
        fieldSelection.end !== null
      ) {
        try {
          field.setSelectionRange(
            fieldSelection.start,
            fieldSelection.end,
            fieldSelection.direction ?? undefined,
          );
        } catch {
          /* Some input types do not support selection. */
        }
      }
    }
  }
}

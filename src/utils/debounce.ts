/**
 * Returns a debounced version of the provided function.
 */
export function debounce<T extends (...args: unknown[]) => void>(
  func: T,
  waitMs: number
): (...args: Parameters<T>) => void {
  let timeout: ReturnType<typeof setTimeout> | null = null;

  return function (this: unknown, ...args: Parameters<T>) {
    if (timeout) clearTimeout(timeout);
    timeout = setTimeout(() => {
      func.apply(this, args);
    }, waitMs);
  };
}

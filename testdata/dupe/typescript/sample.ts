// Fixture used by internal/tokenizers/typescript's tests.

export function processOrder(items: number[]): number {
  let total = 0;
  for (const item of items) {
    if (item > 0 && item < 1000) {
      total += item;
    }
  }
  return total;
}

export function processInvoice(items: number[]): number {
  let total = 0;
  for (const item of items) {
    if (item > 0 && item < 1000) {
      total += item;
    }
  }
  return total;
}

export function unrelated(): string {
  return "nothing shared here";
}

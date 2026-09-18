// Fixture used by internal/tokenizers/typescript's tests (plain JavaScript).

export function processOrder(items) {
  let total = 0;
  for (const item of items) {
    if (item > 0 && item < 1000) {
      total += item;
    }
  }
  return total;
}

export function processInvoice(items) {
  let total = 0;
  for (const item of items) {
    if (item > 0 && item < 1000) {
      total += item;
    }
  }
  return total;
}

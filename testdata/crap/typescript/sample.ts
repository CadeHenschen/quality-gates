// Fixture used by internal/analyzers/typescript's tests.

export function simple(): number {
  return 1;
}

export function branchy(x: number): number {
  if (x > 0) {
    return x;
  }
  return -x;
}

export function loopy(items: number[]): number {
  let total = 0;
  for (const v of items) {
    if (v > 0 && v < 100) {
      total += v;
    }
  }
  return total;
}

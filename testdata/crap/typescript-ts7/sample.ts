// Minimal fixture for the TS7-fallback test — only needs to parse, its
// exact complexity isn't the point (that's covered by testdata/typescript).
export function branchy(x: number): number {
  if (x > 0) {
    return x;
  }
  return -x;
}

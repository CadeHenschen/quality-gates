export function clean(x: number): number {
  return x * 2;
}

export function withTsIgnore(x: number) {
  // @ts-ignore
  const y: string = x;
  return y;
}

export function withEslintDisable() {
  // eslint-disable-next-line no-console
  console.log("noisy");
}

// @ts-nocheck
export function withTsNocheck(x: string): number {
  return x;
}

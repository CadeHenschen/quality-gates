// Fixture used by internal/analyzers/typescript's tests (plain JavaScript).

export function simple() {
  return 1;
}

export function branchy(x) {
  if (x > 0) {
    return x;
  }
  return -x;
}

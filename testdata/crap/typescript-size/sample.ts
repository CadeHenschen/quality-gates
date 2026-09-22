// Fixture used by internal/analyzers/typescript's size-fact tests.

function simple(): number {
  return 1;
}

function manyParams(a: number, b: number, c: number, d: number): number {
  return a + b + c + d;
}

function elifChain(a: boolean, b: boolean, c: boolean): number {
  if (a) {
    return 1;
  } else if (b) {
    return 2;
  } else if (c) {
    return 3;
  } else {
    return 4;
  }
}

function nested(items: number[]): number {
  let total = 0;
  for (const v of items) {
    if (v > 0) {
      total += v;
    }
  }
  return total;
}

class Widget {
  method(a: number, b: number): number {
    return a + b;
  }
}

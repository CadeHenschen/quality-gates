// Fixture used by internal/analyzers/typescript's tests (JSX).

export function Widget(active) {
  if (active) {
    return <span>on</span>;
  }
  return <span>off</span>;
}

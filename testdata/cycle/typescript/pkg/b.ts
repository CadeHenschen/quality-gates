import { useB } from "./a";

export function helperB(): string {
  return "b";
}

export function callUseB() {
  return useB();
}

import { useB } from "./pkg/a";
import external from "react";

export function main() {
  return useB() + String(external);
}

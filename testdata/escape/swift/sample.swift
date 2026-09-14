import Foundation

func clean(_ x: Int) -> Int {
    return x * 2
}

func forceTry() throws -> Int {
    let value = try! JSONDecoder().decode(Int.self, from: Data())
    return value
}

func forceCast(_ any: Any) -> Int {
    let n = any as! Int
    return n
}

func forceUnwrapOnly(_ x: Int?) -> Int {
    let y = x!
    return y
}

func discardedError(_ x: String) -> Int? {
    let n = try? Int(x)
    return n
}

// swiftlint:disable force_unwrapping
func suppressed() {}

func notEqualIsNotForceUnwrap(_ a: Int, _ b: Int) -> Bool {
    return a != b
}

func prefixNotIsNotForceUnwrap(_ flag: Bool) -> Bool {
    return !flag
}

import Foundation

struct Calculator {
    var total: Int = 0

    // Simple branching: complexity 1 + 2 (if, &&) = 3.
    func classify(_ n: Int, positive: Bool) -> String {
        if n > 0 && positive {
            return "positive"
        }
        return "other"
    }

    // A nested local func's branches fold into the enclosing function's
    // complexity — this is the sharpest correctness case for the analyzer.
    // Enclosing: +1 (if) = 2. Nested helper: +1 (if) more = 3 total, and
    // must NOT appear as its own crap.Function.
    func withNestedHelper(_ n: Int) -> Int {
        func double(_ x: Int) -> Int {
            if x < 0 {
                return 0
            }
            return x * 2
        }
        if n > 100 {
            return double(n)
        }
        return n
    }

    // Computed property accessors are not analyzed as functions in v1 —
    // this must not appear as its own crap.Function and must not corrupt
    // neighboring functions' line ranges.
    var doubled: Int {
        get {
            if total < 0 {
                return 0
            }
            return total * 2
        }
        set {
            total = newValue / 2
        }
    }

    func plain() -> Int {
        return total
    }
}

extension Calculator {
    func reset() {
        total = 0
    }
}

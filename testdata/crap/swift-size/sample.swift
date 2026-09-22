// Fixture used by internal/analyzers/swift's size-fact tests.
struct Sample {
    func simple() -> Int {
        return 1
    }

    func manyParams(a: Int, b: Int, c: Int, d: Int) -> Int {
        return a + b + c + d
    }

    func elifChain(a: Bool, b: Bool, c: Bool) -> Int {
        if a {
            return 1
        } else if b {
            return 2
        } else if c {
            return 3
        } else {
            return 4
        }
    }

    func nested(_ items: [Int]) -> Int {
        var total = 0
        for v in items {
            if v > 0 {
                total += v
            }
        }
        return total
    }

    func guarded(_ x: Int?) -> Int {
        guard let x = x else {
            return 0
        }
        return x
    }

    func withNestedHelper(_ n: Int) -> Int {
        func helper(_ x: Int) -> Int {
            if x < 0 {
                if x < -10 {
                    return -1
                }
                return 0
            }
            return x
        }
        return helper(n)
    }
}

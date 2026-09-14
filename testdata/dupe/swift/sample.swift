// A fixture with a deliberate duplicate block, used by
// internal/tokenizers/swift and internal/dupe's tests.

func processOrder(_ items: [Int]) -> Int {
    var total = 0
    for v in items {
        if v > 0 && v < 1000 {
            total += v
        }
        if v > total {
            total = v
        }
    }
    return total
}

func processInvoice(_ items: [Int]) -> Int {
    var total = 0
    for v in items {
        if v > 0 && v < 1000 {
            total += v
        }
        if v > total {
            total = v
        }
    }
    return total
}

func unrelated() -> String {
    return "nothing shared here"
}

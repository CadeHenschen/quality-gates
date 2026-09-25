import Foundation

struct Snapshot {
    var effectiveMl: Double
    var goalMl: Double

    // A bare implicit-getter computed property with no "get"/"set" — no
    // stored backing at all. This is the exact shape that went completely
    // unmeasured before computed properties were analyzed: complexity 1 +
    // 1 (ternary's "?" isn't counted, but the "&&" is) = 2.
    var progress: Double {
        goalMl > 0 && effectiveMl > 0 ? min(1, effectiveMl / goalMl) : 0
    }

    // An explicit get/set with a named setter parameter, exercising the
    // parenthesized-accessor-parameter path (as opposed to bare "set" with
    // the implicit "newValue").
    var scaled: Double {
        get { effectiveMl * 2 }
        set(newScaled) {
            if newScaled >= 0 {
                effectiveMl = newScaled / 2
            }
        }
    }

    // A stored property with a didSet observer: the observer is analyzed
    // as "Snapshot.total.didSet", the stored declaration itself is not.
    var total: Int = 0 {
        didSet {
            if total < 0 {
                total = 0
            }
        }
    }

    // A stored property whose default value happens to be a closure
    // literal — not an accessor block (no get/set/willSet/didSet), so it
    // must not be attributed to this property at all, and must not
    // corrupt the range of the plain function that follows it.
    var transform: (Int) -> Int = { x in
        if x < 0 {
            return 0
        }
        return x
    }

    func plain() -> Int {
        return total
    }
}

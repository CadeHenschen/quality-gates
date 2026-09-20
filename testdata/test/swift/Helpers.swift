import XCTest

func assertBalanced(_ n: Int) {
    XCTAssertEqual(n % 2, 1)
}

// locate's name doesn't sound like an assertion, but its body asserts.
func locate(_ name: String) {
    XCTAssertFalse(name.isEmpty)
}

func outer() {
    locate("y")
}

func makeFixture() {
    _ = 1
}

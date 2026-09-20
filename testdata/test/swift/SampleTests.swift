import XCTest

final class SampleTests: XCTestCase {
    func testGood() {
        XCTAssertEqual(1 + 1, 2)
        XCTAssertTrue(true)
    }

    // A comment mentioning XCTAssertEqual( must not count as an assertion.
    func testNoAssertions() {
        let x = 1 + 1
        _ = x
    }

    func testSetupHelperIsNotAnAssertion() {
        makeFixture()
    }

    func testDelegatesToNamedHelper() {
        assertBalanced(3)
    }

    func testDelegatesToUnnamedHelper() {
        locate("x")
    }

    func testDelegatesToHelperOfHelper() {
        outer()
    }

    func testUnconditionalSkip() throws {
        throw XCTSkip("later")
    }

    func testConditionalSkipIsFine() throws {
        guard ProcessInfo.processInfo.environment["TOOL"] != nil else {
            throw XCTSkip("tool missing")
        }
        XCTAssertTrue(true)
    }

    func testSkipIfIsFine() throws {
        try XCTSkipIf(true, "nope")
        XCTAssertTrue(true)
    }

    func testExpectedFailure() {
        XCTExpectFailure("known bug")
        XCTAssertEqual(1, 2)
    }

    func testExpectationIsNotAnAssertion() {
        let e = expectation(description: "done")
        e.fulfill()
        wait(for: [e], timeout: 1)
    }

    func testNestedLocalFuncFoldsIntoTest() {
        func inner() {
            XCTAssertTrue(true)
        }
        inner()
    }

    func testTempNoCleanup() throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("x")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        XCTAssertTrue(true)
    }

    func testTempWithCleanup() throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("x")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: dir) }
        XCTAssertTrue(true)
    }

    func testHasParametersIsNotATest(x: Int) {
    }

    func helperNotATest() {
    }
}

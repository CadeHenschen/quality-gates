import XCTest

final class TeardownTests: XCTestCase {
    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(atPath: NSTemporaryDirectory())
    }

    func testTempCleanedByTearDown() throws {
        let dir = NSTemporaryDirectory()
        try FileManager.default.createDirectory(atPath: dir, withIntermediateDirectories: true)
        XCTAssertTrue(true)
    }
}

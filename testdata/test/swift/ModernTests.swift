import Testing

struct ModernTests {
    @Test func good() {
        #expect(1 + 1 == 2)
    }

    @Test("has no assertions", .tags(.slow))
    func noAssertions() {
        let x = 1
        _ = x
    }

    @Test(.disabled("flaky"))
    func disabled() {
        #expect(true)
    }

    @Test(.enabled(if: false))
    func conditionallyEnabledIsFine() {
        #expect(true)
    }

    @Test func requireCounts() throws {
        let v = try #require(Optional(1))
        _ = v
    }

    @Test func issueRecordCounts() {
        Issue.record("boom")
    }

    @Test func knownIssue() {
        withKnownIssue {
            #expect(1 == 2)
        }
    }

    func notATestWithoutAttribute() {
    }
}

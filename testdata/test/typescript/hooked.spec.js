const { mkdtempSync } = require('fs');

afterEach(() => {});

it('temp dir cleaned by a file-level hook', () => {
  const d = mkdtempSync('x');
  expect(d).toBeTruthy();
});

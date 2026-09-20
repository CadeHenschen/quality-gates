import { mkdtempSync, rmSync } from 'fs';

describe('good suite', () => {
  it('asserts a value', () => {
    expect(1 + 1).toBe(2);
  });

  it('has no assertions', () => {
    const x = 1 + 1;
  });

  it('delegates to a helper', () => {
    assertSomething(1);
  });

  it('uses node assert', () => {
    assert.equal(1, 1);
    assert(true);
  });

  it('only verifies mocks', () => {
    const fn = jest.fn();
    expect(fn).toHaveBeenCalled();
    expect(fn).not.toHaveBeenCalledWith(1);
  });

  it('mock plus behavior', () => {
    const fn = jest.fn();
    expect(fn).toHaveBeenCalled();
    expect(fn.mock.results.length).toBe(0);
  });

  it.skip('is skipped', () => {
    expect(1).toBe(1);
  });

  xit('is x-skipped', () => {});

  it.todo('write this later');

  it.only('is focused', () => {
    expect(1).toBe(1);
  });

  it.each([1, 2])('table %i', (n) => {
    expect(n).toBeTruthy();
  });

  it('declares expect.assertions but asserts nothing else', () => {
    expect.assertions(1);
  });

  it('leaks a temp dir', () => {
    const d = mkdtempSync('x');
    expect(d).toBeTruthy();
  });

  it('cleans a temp dir', () => {
    const d = mkdtempSync('x');
    expect(d).toBeTruthy();
    rmSync(d, { recursive: true });
  });
});

describe.skip('skipped suite', () => {
  it('inner', () => {
    expect(1).toBe(1);
  });
});

fdescribe('focused suite', () => {});

test('top-level test', () => {
  expect(true).toBe(true);
});

test.skipIf(process.platform === 'win32')('conditional skip is fine', () => {
  expect(true).toBe(true);
});

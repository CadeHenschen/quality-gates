# Fixture used by internal/analyzers/python's tests.


def simple():
    return 1


def branchy(x):
    if x > 0:
        return x
    return -x


class Widget:
    def loopy(self, items):
        total = 0
        for v in items:
            if 0 < v < 100:
                total += v
        return total

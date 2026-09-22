# Fixture used by internal/analyzers/python's size-fact tests.


def simple():
    return 1


def many_args(a, b, c, *args, **kwargs):
    return a + b + c


def elif_chain(a, b, c):
    if a:
        return 1
    elif b:
        return 2
    elif c:
        return 3
    else:
        return 4


def nested(items):
    total = 0
    for v in items:
        if v > 0:
            total += v
    return total


class Widget:
    def method(self, a, b):
        return a + b

    @staticmethod
    def helper(a, b):
        return a + b

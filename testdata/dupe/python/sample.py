# Fixture used by internal/tokenizers/python's tests.


def process_order(items):
    total = 0
    for item in items:
        if item > 0 and item < 1000:
            total += item
    return total


def process_invoice(items):
    total = 0
    for item in items:
        if item > 0 and item < 1000:
            total += item
    return total


def unrelated():
    return "nothing shared here"

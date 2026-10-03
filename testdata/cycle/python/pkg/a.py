from . import (
    # Local modules can be listed over several lines.
    b as imported_b,
    c,
)


def use_b():
    return imported_b.helper() + c.label()

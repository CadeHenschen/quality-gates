def clean(x):
    return x * 2


def with_type_ignore(x):
    y = x + "1"  # type: ignore
    return y


def with_noqa():
    import os  # noqa

    return os


def with_bare_except():
    try:
        risky()
    except:
        pass


def with_pylint_disable(x):
    eval(x)  # pylint: disable=eval-used
    return x

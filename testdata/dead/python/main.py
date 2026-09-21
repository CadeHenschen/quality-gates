import os
import sys

from pkg.util import used


def unused_function():
    return 1


class Dead:
    unused_attr = 1

    def method(self):
        unused_local = 2
        return 3


def main(argv):
    if False:
        return 1
    return used()


if __name__ == "__main__":
    main(sys.argv)

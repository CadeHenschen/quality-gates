import os
import shutil
import tempfile
import unittest
from unittest import mock

import pytest


def test_good():
    assert 1 + 1 == 2


def test_no_assertions():
    x = 1 + 1


def test_setup_helper_is_not_an_assertion():
    make_fixture()


def test_delegates_to_helper():
    assert_frame_equal(1, 1)


def test_raises():
    with pytest.raises(ValueError):
        int("x")


@pytest.mark.skip(reason="later")
def test_unconditional_skip():
    assert True


@pytest.mark.skipif(os.name == "nt", reason="posix only")
def test_conditional_skip_is_fine():
    assert True


@pytest.mark.xfail
def test_expected_failure():
    assert False


def test_skip_call():
    pytest.skip("later")
    assert True


def test_skip_call_in_guard_is_fine():
    if os.name == "nt":
        pytest.skip("posix only")
    assert True


def test_only_mock_assertions():
    m = mock.Mock()
    m.assert_called_once()
    assert m.call_count == 1


def test_mock_plus_behavior():
    m = mock.Mock()
    m.assert_called_once()
    assert m.return_value is not None


def test_temp_no_cleanup():
    d = tempfile.mkdtemp()
    assert d


def test_temp_with_cleanup():
    d = tempfile.mkdtemp()
    assert d
    shutil.rmtree(d)


def test_temp_with_finally():
    d = tempfile.mkdtemp()
    try:
        assert d
    finally:
        os.rmdir(d)


def helper_not_a_test():
    pass


class TestThing:
    def test_method(self):
        assert True

    def test_method_no_asserts(self):
        pass


@unittest.skip("whole class")
class TestSkippedClass(unittest.TestCase):
    def test_a(self):
        self.assertEqual(1, 1)


class TestCaseStyle(unittest.TestCase):
    def test_assert_style(self):
        self.assertEqual(1, 1)
        self.assertTrue(True)

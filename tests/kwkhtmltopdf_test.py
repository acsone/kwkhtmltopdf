#!/usr/bin/env python

"""
Before running these tests,
* start kwkhtmltopdf_server
* set KWKHTMLTOPDF_SERVER_URL environment variable
"""

import os
import re
import subprocess

import pytest
from wand.image import Image

HERE = os.path.dirname(__file__)


def _wkhtmltopdf_bin():
    return os.getenv("KWKHTMLTOPDF_BIN", "wkhtmltopdf")


class Client:
    def __init__(self, cmd):
        self.cmd = cmd

    def _run_stdout(self, args, check_return_code=True):
        proc = subprocess.Popen(
            self.cmd + args,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            universal_newlines=True,
        )
        out, _ = proc.communicate()
        if check_return_code:
            assert proc.returncode == 0, out
        return out

    def _run_expect_error(self, args):
        r = subprocess.call(self.cmd + args)
        assert r != 0

    def _run_expect_file(self, args, expected_data_file):
        r = subprocess.call(self.cmd + args, cwd=os.path.join(HERE, "data"))
        assert r == 0
        expected = Image(filename=os.path.join(HERE, "data", expected_data_file))
        actual = Image(filename=str(args[-1]))
        diff = actual.compare(expected, metric="root_mean_square")
        assert diff[1] < 0.01


@pytest.fixture(
    params=["native", "client", "client_sys_py2", "client_sys_py3"], scope="module"
)
def client(request):
    if request.param == "native":
        yield Client([_wkhtmltopdf_bin()])
    elif request.param == "client":
        # run the client with same python as test suite
        yield Client(
            [os.path.join(HERE, "..", "client", "python", "kwkhtmltopdf_client.py")]
        )
    elif request.param == "client_sys_py2":
        # run the client with the system python2
        yield Client(
            [
                "/usr/bin/python2",
                os.path.join(HERE, "..", "client", "python", "kwkhtmltopdf_client.py"),
            ]
        )
    elif request.param == "client_sys_py3":
        # run the client with the system python2
        yield Client(
            [
                "/usr/bin/python3",
                os.path.join(HERE, "..", "client", "python", "kwkhtmltopdf_client.py"),
            ]
        )


def test_noargs(client):
    out = client._run_stdout([], check_return_code=False)
    assert "Synopsis:" in out
    assert "Headers And Footer Options:" not in out


def test_help(client):
    out = client._run_stdout(["-h"])
    assert "Synopsis:" in out
    assert "Headers And Footer Options:" not in out


def test_extended_help(client):
    out = client._run_stdout(["--extended-help"])
    assert "Synopsis:" in out
    assert "Headers And Footer Options:" in out


def test_version(client):
    out = client._run_stdout(["--version"])
    assert re.search(r"wkhtmltopdf [\d\.]+ ", out)


def test_1(client, tmp_path):
    client._run_expect_file(["test1.html", tmp_path / "o.pdf"], "test1.pdf")


def test_bad_option(client):
    client._run_expect_error(["--not-an-option"])


def test_not_found(client, tmp_path):
    client._run_expect_error(["https://github.com/acsone/notfound", tmp_path / "o.pdf"])


def test_image_not_found(client, tmp_path):
    client._run_expect_file(
        ["--load-media-error-handling", "ignore", "test2.html", tmp_path / "o.pdf"],
        "test2.pdf",
    )
    client._run_expect_error(
        ["--load-media-error-handling", "abort", "test2.html", tmp_path / "o.pdf"]
    )

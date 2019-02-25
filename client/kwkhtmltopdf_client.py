#!/usr/bin/env python
# Copyright (c) 2019 ACSONE SA/NV
# Distributed under the MIT License (http://opensource.org/licenses/MIT)

import os
import sys

import requests


CHUNK_SIZE = 8192


def wkhtmltopdf(args):
    url = os.getenv("KWKHTMLTOPDF_SERVER_URL")
    parts = []

    def add_option(option):
        # TODO option encoding?
        parts.append(("option", (None, option)))

    def add_file(filename):
        with open(filename, "rb") as f:
            parts.append(("file", (filename, f.read())))

    output = "-"
    if args and not args[-1].startswith("-"):
        output = args[-1]
        args = args[:-1]

    for arg in args:
        if arg.startswith("-"):
            add_option(arg)
        elif arg.startswith("http://") or arg.startswith("https://"):
            add_option(arg)
        elif arg.startswith("file://"):
            add_file(arg[7:])
        elif os.path.isfile(arg):
            # TODO better way to detect args that are actually options
            # TODO in case an option has the same name as an existing file
            # TODO only way I see so far is enumerating them in a static
            # TODO datastructure (that can be initialized with a quick parse
            # TODO of wkhtmltopdf --extended-help)
            add_file(arg)
        else:
            add_option(arg)

    if not parts:
        add_option("-h")

    r = requests.post(url, files=parts)
    r.raise_for_status()
    # TODO sys.stdout.buffer does not work with python2
    with (sys.stdout.buffer if output == "-" else open(output, "wb")) as f:
        for chunk in r.iter_content(chunk_size=CHUNK_SIZE):
            f.write(chunk)


if __name__ == "__main__":
    wkhtmltopdf(sys.argv[1:])

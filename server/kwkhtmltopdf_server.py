#!/usr/bin/env python3
# Copyright (c) 2019 ACSONE SA/NV
# Distributed under the MIT License (http://opensource.org/licenses/MIT)

import asyncio
import os
import shutil
import sys
import tempfile

from aiohttp import web


CHUNK_SIZE = 2 ** 16


def _wkhtmltopdf_bin():
    return os.getenv("KWKHTMLTOPDF_BIN", "wkhtmltopdf")


def _is_pdf_command(args):
    args = set(args)
    no_pdf_options = set(("-V", "--version", "-h", "--help", "-H", "--extended-help"))
    return not bool(args & no_pdf_options)


async def wkhtmltopdf(request):
    # TODO handle empty requests (without args)
    reader = await request.multipart()
    args = []
    tmpdir = tempfile.mkdtemp()
    try:
        # read arguments, store files in temporary files
        while True:
            part = await reader.next()
            if not part:
                break
            if part.name == "option":
                option = await part.text()
                if not option:
                    continue
                args.append(option)
            elif part.name == "file":
                assert part.filename
                # It's important to preserve as much as possible of the
                # original filename because some javascript can depend on it
                # through document.location.
                filename = os.path.join(tmpdir, os.path.basename(part.filename))
                # TODO what if multiple files with same basename?
                assert not os.path.exists(filename)
                with open(filename, "wb") as f:
                    while True:
                        chunk = await part.read_chunk(CHUNK_SIZE)
                        if not chunk:
                            break
                        f.write(chunk)
                    args.append(filename)
        is_pdf_command = _is_pdf_command(args)
        # run wkhtmltopdf and stream response
        if is_pdf_command:
            args.append("-")
        cmd = [_wkhtmltopdf_bin()] + args
        print(">", " ".join(cmd), file=sys.stderr)
        proc = await asyncio.create_subprocess_exec(
            *cmd, stdout=asyncio.subprocess.PIPE
        )
        response = web.StreamResponse(status=200)
        response.enable_chunked_encoding()
        if is_pdf_command:
            response.content_type = "application/pdf"
        else:
            response.content_type = "text/plain"
        await response.prepare(request)
        while True:
            chunk = await proc.stdout.read()
            if not chunk:
                break
            await response.write(chunk)
        r = await proc.wait()
        if r != 0:
            raise web.HTTPException()
        print("<", " ".join(cmd), file=sys.stderr)
        return response
    finally:
        shutil.rmtree(tmpdir)


if __name__ == "__main__":
    app = web.Application()
    app.add_routes([web.post("/", wkhtmltopdf)])
    web.run_app(app)

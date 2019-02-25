#!/usr/bin/env python3
# Copyright (c) 2019 ACSONE SA/NV
# Distributed under the MIT License (http://opensource.org/licenses/MIT)

import asyncio
import os
import shutil
import sys
import tempfile

from aiohttp import web


CHUNK_SIZE = 8192


def _wkhtmltopdf_bin():
    return os.getenv("KWKHTMLTOPDF_BIN", "wkhtmltopdf")


async def wkhtmltopdf_form(request):
    # TODO improve the look of this form
    form = """\
        <htmt>
            <head>
                <title>wkhtmltopdf server</title>
            </head>
            <body>
                <h1>wkhtmltopdf server</h1>
                <form action="/" method="post"
                      enctype="multipart/form-data">
                    Options<br>
                    <input name="option" type="text" value="-O"><br>
                    <input name="option" type="text" value="portrait"><br>
                    <input name="option" type="text"><br>
                    <input name="option" type="text"><br>
                    <input name="option" type="text"><br>
                    <input name="option" type="text"><br>
                    Url to convert<br>
                    <input name="option" type="text"
                           value="https://wkhtmltopdf.org" autofocus><br>
                    File to convert<br>
                    <input name="file" type="file"><br>
                    <br>
                    <input type="submit" value="Convert to PDF!"><br>
                </form>
                <i>
                    Brought to you by
                    <a href="https://acsone.eu">ACSONE SA/NV</href>.
                </i>
            </body>
        </html>
    """
    return web.Response(text=form, content_type="text/html")


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
                # TODO validate filename?
                assert part.filename
                # It's important to preserve as much as possible of the
                # original filename because some javascript can depend on it
                # through document.location.
                # TODO what if multiple files with same basename?
                filename = os.path.join(tmpdir, os.path.basename(part.filename))
                with open(filename, "wb") as f:
                    while True:
                        chunk = await part.read_chunk(CHUNK_SIZE)
                        if not chunk:
                            break
                        f.write(chunk)
                    args.append(filename)
        with tempfile.NamedTemporaryFile(suffix=".pdf", delete=False, dir=tmpdir) as f:
            output = f.name
        # run wkhtmltopdf
        cmd = [_wkhtmltopdf_bin()] + args + [output]
        proc = await asyncio.create_subprocess_exec(
            *cmd, stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE
        )
        stdout_data, stderr_data = await proc.communicate()
        r = proc.returncode
        print(" ".join(cmd), file=sys.stderr)
        sys.stderr.buffer.write(stderr_data)
        print("=>", r, file=sys.stderr)
        # return result
        if r != 0:
            # error
            response = web.Response(text=stderr_data.decode(), status=400)
        elif not os.path.exists(output) or not os.path.getsize(output):
            # version or help
            response = web.Response(text=stdout_data.decode(), status=200)
        else:
            # pdf
            # TODO stream wkhtmltopdf output directly to response,
            # TODO but then how to report errors?
            # TODO With chunked encoding, closing connection informs the client
            # TODO that something is wrong, but there is no way to report
            # TODO an error message (except trailer headers, maybe, not
            # TODO sure if many HTTP clients support that).
            response = web.StreamResponse(status=200)
            response.enable_chunked_encoding()
            response.content_type = "application/pdf"
            await response.prepare(request)
            with open(output, "rb") as f:
                while True:
                    chunk = f.read(CHUNK_SIZE)
                    await response.write(chunk)
                    if len(chunk) != CHUNK_SIZE:
                        break
        return response
    finally:
        shutil.rmtree(tmpdir)


if __name__ == "__main__":
    app = web.Application()
    app.add_routes([web.post("/", wkhtmltopdf), web.get("/", wkhtmltopdf_form)])
    web.run_app(app)

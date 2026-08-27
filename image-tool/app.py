import io
import json
import re
import threading
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

from PIL import Image, ImageChops, ImageOps
from rembg import new_session, remove


MAX_INPUT_BYTES = 8 * 1024 * 1024
MAX_OUTPUT_EDGE = 2000
Image.MAX_IMAGE_PIXELS = 25_000_000
SESSION = new_session("u2net_human_seg")
INFERENCE_SLOTS = threading.BoundedSemaphore(2)


def render_id_photo(source, width, height, background):
    original = ImageOps.exif_transpose(Image.open(io.BytesIO(source))).convert("RGB")
    if original.width < 80 or original.height < 80:
        raise ValueError("image is too small")

    with INFERENCE_SLOTS:
        foreground = remove(original, session=SESSION).convert("RGBA")

    alpha = foreground.getchannel("A")
    visible = alpha.point(lambda value: 255 if value >= 12 else 0)
    bounds = visible.getbbox()
    if bounds is None:
        raise ValueError("no person detected")

    subject = foreground.crop(bounds)
    subject_alpha = subject.getchannel("A")
    subject_width, subject_height = subject.size

    # ID photos need a small head margin and the shoulders close to the lower edge.
    target_subject_width = int(width * 0.90)
    target_subject_height = int(height * 0.95)
    scale = min(target_subject_width / subject_width, target_subject_height / subject_height)
    resized = subject.resize(
        (max(1, round(subject_width * scale)), max(1, round(subject_height * scale))),
        Image.Resampling.LANCZOS,
    )
    resized_alpha = subject_alpha.resize(resized.size, Image.Resampling.LANCZOS)
    resized.putalpha(resized_alpha)

    canvas = Image.new("RGB", (width, height), background)
    x = (width - resized.width) // 2
    y = height - resized.height
    if y < int(height * 0.025):
        y = int(height * 0.025)
    canvas.paste(resized, (x, y), resized)

    output = io.BytesIO()
    canvas.save(output, format="JPEG", quality=95, optimize=True, dpi=(300, 300))
    return output.getvalue()


class Handler(BaseHTTPRequestHandler):
    server_version = "AIWorkbenchImageTool/1.0"

    def do_GET(self):
        if urlparse(self.path).path != "/health":
            self.send_error(HTTPStatus.NOT_FOUND)
            return
        self._json(HTTPStatus.OK, {"status": "ok"})

    def do_POST(self):
        parsed = urlparse(self.path)
        if parsed.path != "/v1/id-photo":
            self.send_error(HTTPStatus.NOT_FOUND)
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
            if length <= 0 or length > MAX_INPUT_BYTES:
                raise ValueError("invalid content length")
            query = parse_qs(parsed.query)
            width = int(query.get("width", [""])[0])
            height = int(query.get("height", [""])[0])
            color = query.get("background", [""])[0].lower()
            if not (80 <= width <= MAX_OUTPUT_EDGE and 80 <= height <= MAX_OUTPUT_EDGE):
                raise ValueError("invalid dimensions")
            if not re.fullmatch(r"[0-9a-f]{6}", color):
                raise ValueError("invalid background")
            source = self.rfile.read(length)
            result = render_id_photo(source, width, height, "#" + color)
        except (ValueError, OSError, Image.DecompressionBombError) as error:
            self._json(HTTPStatus.BAD_REQUEST, {"error": str(error)[:200]})
            return
        except Exception:
            self._json(HTTPStatus.INTERNAL_SERVER_ERROR, {"error": "image processing failed"})
            return

        self.send_response(HTTPStatus.OK)
        self.send_header("Content-Type", "image/jpeg")
        self.send_header("Content-Length", str(len(result)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(result)

    def log_message(self, message, *args):
        print("%s - %s" % (self.address_string(), message % args), flush=True)

    def _json(self, status, payload):
        body = json.dumps(payload, ensure_ascii=True).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8090), Handler).serve_forever()

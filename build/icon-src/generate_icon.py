#!/usr/bin/env python3
"""Generate Kubikles app icons (SVG, PNG, ICO, ICNS)."""

import math
import os
import shutil
import subprocess
import sys
import tempfile

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
BUILD_DIR = os.path.dirname(SCRIPT_DIR)

SIZE = 1024
PADDING = 0
BG_COLOR = "#1a1d23"
ACCENT = "#007acc"
ACCENT_LIGHT = "#3ba0e6"
ACCENT_DIM = "#1e5a8a"     # Visible spoke color against dark bg
WHITE = "#e8eaed"

cx, cy = SIZE // 2, SIZE // 2
corner_r = 228              # macOS Big Sur standard continuous corner radius

# Wheel/cluster params
outer_r = 280
spoke_count = 7
spoke_width = 14
hub_r = 120
hub_ring_width = 20
ring_width = 7
tip_r = 38
spoke_tip_r = 340

def polar(cx, cy, r, angle_deg):
    rad = math.radians(angle_deg)
    return cx + r * math.cos(rad), cy + r * math.sin(rad)

def make_icon_svg(include_text=False, inapp=False):
    lines = []
    content_r = spoke_tip_r + tip_r + 16  # radius covering all content including glow

    if include_text:
        w, h = 1024, 810
        icon_cy = 360
    elif inapp:
        # Tight viewBox around content only
        w = h = content_r * 2
        icon_cy = content_r
    else:
        w, h = SIZE, SIZE
        icon_cy = cy

    # For inapp, shift center to match the cropped viewBox
    icon_cx = content_r if inapp else cx

    lines.append(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h}" width="{w}" height="{h}">')

    # Background
    if include_text:
        lines.append(f'  <rect x="0" y="0" width="{w}" height="{h}" fill="{BG_COLOR}"/>')
    elif inapp:
        lines.append(f'  <rect x="0" y="0" width="{w}" height="{h}" fill="{BG_COLOR}"/>')
    else:
        lines.append(f'  <rect x="{PADDING}" y="{PADDING}" width="{SIZE - 2*PADDING}" height="{SIZE - 2*PADDING}" rx="{corner_r}" ry="{corner_r}" fill="{BG_COLOR}"/>')

    # Outer ring connecting the nodes
    lines.append(f'  <circle cx="{icon_cx}" cy="{icon_cy}" r="{spoke_tip_r}" fill="none" stroke="{ACCENT_DIM}" stroke-width="{ring_width}" stroke-dasharray="12 8" opacity="0.7"/>')

    # Connection spokes from hub to nodes
    for i in range(spoke_count):
        angle = (360 / spoke_count) * i - 90
        x1, y1 = polar(icon_cx, icon_cy, hub_r, angle)
        x2, y2 = polar(icon_cx, icon_cy, spoke_tip_r, angle)
        lines.append(f'  <line x1="{x1:.1f}" y1="{y1:.1f}" x2="{x2:.1f}" y2="{y2:.1f}" stroke="{ACCENT}" stroke-width="{spoke_width}" stroke-linecap="round" opacity="0.8"/>')

    # Node dots at spoke tips
    for i in range(spoke_count):
        angle = (360 / spoke_count) * i - 90
        tx, ty = polar(icon_cx, icon_cy, spoke_tip_r, angle)
        # Glow
        lines.append(f'  <circle cx="{tx:.1f}" cy="{ty:.1f}" r="{tip_r + 12}" fill="{ACCENT}" opacity="0.2"/>')
        # Dot
        lines.append(f'  <circle cx="{tx:.1f}" cy="{ty:.1f}" r="{tip_r}" fill="{ACCENT_LIGHT}"/>')

    # Hub - solid accent fill
    lines.append(f'  <circle cx="{icon_cx}" cy="{icon_cy}" r="{hub_r}" fill="{ACCENT}"/>')
    lines.append(f'  <circle cx="{icon_cx}" cy="{icon_cy}" r="{hub_r - hub_ring_width}" fill="{BG_COLOR}"/>')

    # "K" letter
    lines.append(f'  <text x="{icon_cx}" y="{icon_cy + 40}" text-anchor="middle" font-family="-apple-system, SF Pro Display, Helvetica Neue, Arial, sans-serif" font-weight="900" font-size="130" fill="{WHITE}">K</text>')

    if include_text:
        lines.append(f'  <text x="{icon_cx}" y="{h - 45}" text-anchor="middle" font-family="-apple-system, SF Pro Display, Helvetica Neue, Arial, sans-serif" font-weight="600" font-size="64" fill="{WHITE}" opacity="0.9" letter-spacing="3">KUBIKLES</text>')

    lines.append('</svg>')
    return '\n'.join(lines)

def find_magick():
    for cmd in ("magick", "convert"):
        if shutil.which(cmd):
            return cmd
    return None

def magick(args):
    cmd = find_magick()
    if not cmd:
        print("ERROR: ImageMagick not found. Install it (brew install imagemagick) for PNG/ICO/ICNS output.", file=sys.stderr)
        sys.exit(1)
    subprocess.run([cmd] + args, check=True)

def generate_png(svg_path, out_path, size=1024, crop_padding=False):
    # -background none MUST come before the input so SVG rasterizes with transparency
    args = ["-background", "none", svg_path, "-resize", f"{size}x{size}", "-gravity", "center"]
    if crop_padding:
        # Trim transparent padding so the icon fills the image edge-to-edge
        args += ["-trim", "+repage"]
    args.append(out_path)
    magick(args)

def generate_ico(svg_path, out_path):
    magick(["-background", "none", svg_path, "-resize", "256x256", "-gravity", "center", out_path])

def generate_icns(svg_path, out_path):
    with tempfile.TemporaryDirectory(suffix=".iconset") as iconset:
        for size in (16, 32, 128, 256, 512):
            generate_png(svg_path, os.path.join(iconset, f"icon_{size}x{size}.png"), size)
            generate_png(svg_path, os.path.join(iconset, f"icon_{size}x{size}@2x.png"), size * 2)
        subprocess.run(["iconutil", "-c", "icns", iconset, "-o", out_path], check=True)


# --- Generate SVGs ---
icon_svg = os.path.join(SCRIPT_DIR, "icon.svg")
logo_svg = os.path.join(SCRIPT_DIR, "logo.svg")

with open(icon_svg, 'w') as f:
    f.write(make_icon_svg(include_text=False))
with open(logo_svg, 'w') as f:
    f.write(make_icon_svg(include_text=True))
print("SVGs generated")

# --- Generate raster formats ---
if not find_magick():
    print("ImageMagick not found, skipping PNG/ICO/ICNS generation.", file=sys.stderr)
    sys.exit(0)

generate_png(icon_svg, os.path.join(BUILD_DIR, "appicon.png"))
print("appicon.png generated")

# In-app icon: tight circular crop, dark circle bg, CSS rounds it
inapp_svg = os.path.join(SCRIPT_DIR, "icon-inapp.svg")
with open(inapp_svg, 'w') as f:
    f.write(make_icon_svg(inapp=True))
generate_png(inapp_svg, os.path.join(BUILD_DIR, "..", "frontend", "src", "assets", "images", "logo-universal.png"))
print("logo-universal.png generated")

generate_ico(icon_svg, os.path.join(BUILD_DIR, "windows", "icon.ico"))
print("icon.ico generated")

if shutil.which("iconutil"):
    generate_icns(icon_svg, os.path.join(BUILD_DIR, "bin", "kubikles.app", "Contents", "Resources", "iconfile.icns"))
    print("iconfile.icns generated")
else:
    print("iconutil not found (not macOS?), skipping ICNS generation.", file=sys.stderr)

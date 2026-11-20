#!/usr/bin/env bash
# Renders the example GIFs shown in the README and the guide from every image
# in a folder (docs/examples/images unless one is given), in name order:
#
#   docs/examples/render.sh [image-dir]      # or: mise run docs:examples
#
# The examples that need one image use the first. Run it after a change that
# affects how clips look, or after replacing the images.
#
# Each example is a silent seamless loop (--no-audio) with a fixed --seed, so
# the GIFs loop cleanly and re-rendering gives the same result.
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
img="$(cd "${1:-$root/docs/examples/images}" && pwd)"
shopt -s nullglob nocaseglob
images=("$img"/*.{png,jpg,jpeg})
shopt -u nullglob nocaseglob
if ((${#images[@]} == 0)); then
	echo "no .png or .jpg images in $img" >&2
	exit 1
fi
mapfile -t images < <(printf '%s\n' "${images[@]}" | sort)
first="${images[0]}"

cd "$root"
bin="$(mktemp -d)/nltl-animate"
go build -o "$bin" ./cmd/nltl-animate
work="$(mktemp -d)"
stills="$(mktemp -d)"
trap 'rm -rf "$work" "$stills" "$(dirname "$bin")"' EXIT

out=docs/examples
size=270 # GIF width and height in pixels
fps=12

# gif NAME [nltl-animate args...]: render a clip and turn it into NAME.gif.
gif() {
	local name=$1
	shift
	rm -rf "${work:?}"/*
	"$bin" --no-audio --seed 1 --grade vivid --output-dir "$work" "$@" >/dev/null
	ffmpeg -loglevel error -y -i "$work"/*.mp4 \
		-vf "fps=$fps,scale=$size:$size:flags=lanczos,split[a][b];[a]palettegen=max_colors=64[p];[b][p]paletteuse=dither=none" \
		-loop 0 "$out/$name.gif"
	echo "$out/$name.gif"
}

gif single --motion restless "$first"
gif cycle --bpm 120 "${images[@]}"
gif grid grid --n 3 --tiles mixed --bpm 120 "${images[@]}"
gif train train --bpm 120 --vanish 0.75 --offset 1 --bg '#243447' "${images[@]}"
gif shade grid --n 3 --motion shifting --bpm 120 "$first"

# grades.jpg: one still frame of the same image under several grades, side
# by side and labeled.
grades=(clean muted dark mono dream tunnel)
inputs=()
for g in "${grades[@]}"; do
	rm -rf "${work:?}"/*
	"$bin" --no-audio --motion still --grade "$g" --output-dir "$work" "$first" >/dev/null
	ffmpeg -loglevel error -y -ss 1 -i "$work"/*.mp4 -frames:v 1 \
		-vf "scale=240:240,pad=240:272:0:0:white,drawtext=text=$g:x=(w-tw)/2:y=244:fontsize=20:fontcolor=black:y_align=font" \
		"$stills/$g.png"
	inputs+=(-i "$stills/$g.png")
done
ffmpeg -loglevel error -y "${inputs[@]}" -filter_complex "hstack=inputs=${#grades[@]}" -q:v 4 "$out/grades.jpg"
echo "$out/grades.jpg"

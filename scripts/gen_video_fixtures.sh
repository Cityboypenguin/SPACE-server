#!/bin/sh
# internal/upload の動画検査のテストに使う小さな MP4 を作り直す。
# 出力は internal/upload/testdata/video/。ffmpeg（libx264, libx265, libmp3lame）が要る。
# 絵は単色、音は無音に近いので、60秒の動画でも数十KBに収まる。
set -eu
cd "$(dirname "$0")/../internal/upload/testdata/video"

V="-c:v libx264 -preset veryfast -pix_fmt yuv420p -g 30"
A="-c:a aac -b:a 8k -ac 1"
FS="-movflags +faststart"
ff() { ffmpeg -hide_banner -loglevel error -y "$@"; }
color() { echo "color=c=gray:s=$1:r=$2:d=$3"; }
tone() { echo "sine=f=440:d=$1:sample_rate=8000"; }

ff -f lavfi -i "$(color 320x180 30 2)" -f lavfi -i "$(tone 2)" $V $A $FS -shortest landscape.mp4
ff -f lavfi -i "$(color 180x320 30 2)" -f lavfi -i "$(tone 2)" $V $A $FS -shortest portrait.mp4
ff -f lavfi -i "$(color 320x180 30 2)" $V $FS noaudio.mp4
ff -f lavfi -i "$(color 64x36 10 60)" -f lavfi -i "$(tone 60)" $V $A $FS -t 60 d60.mp4
ff -f lavfi -i "$(color 64x36 10 61)" -f lavfi -i "$(tone 61)" $V $A $FS -t 61 d61.mp4
ff -f lavfi -i "$(color 1280x720 30 0.2)" $V $FS hd720.mp4
ff -f lavfi -i "$(color 720x1280 30 0.2)" $V $FS hd720_portrait.mp4
ff -f lavfi -i "$(color 1920x1080 30 0.2)" $V $FS fhd1080.mp4
ff -f lavfi -i "$(color 320x180 60 1)" $V $FS fps60.mp4
ff -f lavfi -i "$(color 320x180 30 1)" -c:v libx265 -tag:v hvc1 -pix_fmt yuv420p $FS hevc.mp4
ff -f lavfi -i "$(color 320x180 30 1)" -f lavfi -i "$(tone 1)" $V -c:a libmp3lame -ar 22050 $FS -shortest mp3audio.mp4
ff -f lavfi -i "$(color 320x180 30 1)" $V notfaststart.mp4
ff -f lavfi -i "$(color 320x180 30 1)" $V -movflags frag_keyframe+empty_moov fragmented.mp4
ff -f lavfi -i "$(color 320x180 30 1)" $V -f mov quicktime.mov
ls -la

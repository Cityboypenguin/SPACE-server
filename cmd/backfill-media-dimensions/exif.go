package main

import (
	"bytes"
	"encoding/binary"
)

// ブラウザは既定で EXIF の Orientation を適用して画像を表示する（CSS の
// image-orientation の初期値が from-image）。一方 Go の image.DecodeConfig は
// EXIF を見ないため、回転指定のある写真では縦横が入れ替わった値を返す。
// 表示側が確保する領域と食い違わないよう、ここで同じ解釈を与える。
//
// 対象は JPEG のみ。WebP と PNG も EXIF を持ち得るが、このアプリの画像は
// canvas で再エンコードして書き出しており EXIF を含まない。回転指定が現実に
// 残っているのはブラウザを通していない頃のカメラ画像（JPEG）だけである。

// exifOrientationSwapsAxes は Orientation の値が縦横の入れ替えを伴うかを返す。
// 5〜8 が 90 度系の回転で、幅と高さが入れ替わる。
func exifOrientationSwapsAxes(orientation int) bool {
	return orientation >= 5 && orientation <= 8
}

// jpegExifOrientation は JPEG の APP1 セグメントから Orientation を読む。
// 見つからない・壊れている場合は 0 を返し、呼び出し側は回転なしとして扱う。
func jpegExifOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0 // SOI が無ければ JPEG ではない
	}

	offset := 2
	for offset+4 <= len(data) {
		if data[offset] != 0xFF {
			return 0 // マーカー境界を見失った
		}
		marker := data[offset+1]
		// SOS 以降は圧縮データなので、これ以上メタデータは現れない
		if marker == 0xDA || marker == 0xD9 {
			return 0
		}
		segmentLength := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		if segmentLength < 2 {
			return 0
		}
		payloadStart := offset + 4
		payloadEnd := offset + 2 + segmentLength
		if payloadEnd > len(data) {
			return 0 // ヘッダの読み取り上限で切れている
		}

		if marker == 0xE1 {
			payload := data[payloadStart:payloadEnd]
			if bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
				return tiffOrientation(payload[6:])
			}
		}
		offset = payloadEnd
	}
	return 0
}

// tiffOrientation は EXIF 本体（TIFF ヘッダ以降）から Orientation タグを読む。
func tiffOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 0
	}

	var order binary.ByteOrder
	switch {
	case tiff[0] == 'I' && tiff[1] == 'I':
		order = binary.LittleEndian
	case tiff[0] == 'M' && tiff[1] == 'M':
		order = binary.BigEndian
	default:
		return 0
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return 0 // TIFF のマジックナンバーが合わない
	}

	ifdOffset := int(order.Uint32(tiff[4:8]))
	if ifdOffset < 8 || ifdOffset+2 > len(tiff) {
		return 0
	}

	entryCount := int(order.Uint16(tiff[ifdOffset : ifdOffset+2]))
	const entrySize = 12
	for i := 0; i < entryCount; i++ {
		entryStart := ifdOffset + 2 + i*entrySize
		if entryStart+entrySize > len(tiff) {
			return 0
		}
		entry := tiff[entryStart : entryStart+entrySize]
		if order.Uint16(entry[0:2]) != 0x0112 { // Orientation
			continue
		}
		if order.Uint16(entry[2:4]) != 3 { // SHORT 以外は想定しない
			return 0
		}
		// SHORT は 4 バイトの値領域の先頭に詰められる
		value := int(order.Uint16(entry[8:10]))
		if value < 1 || value > 8 {
			return 0
		}
		return value
	}
	return 0
}

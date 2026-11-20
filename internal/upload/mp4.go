package upload

import (
	"encoding/binary"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
)

// 動画の受け入れで中身を確かめる。
//
// # 何を確かめ、何を確かめないか
//
// 確かめるのはファイルの「構造」まで。コンテナーが MP4 であること、映像が H.264、
// 音声が AAC（無くてもよい）であること、長さ・解像度・フレームレートが上限内で
// あること、そして各サンプルがファイルの範囲内を指していて表が互いに矛盾しない
// こと（＝壊れていないこと）。
//
// 映像を最後までデコードはしない。API サーバーは 1 vCPU で、デコードは投稿のたびに
// 数秒ぶんの CPU を食う。ffmpeg を同梱すればイメージも大きくなる。そのため、
// 表の上では正しく、サンプルの中身（映像データ）だけが壊れているファイルは通る。
// 見た人の画面で再生できないだけで、他の利用者やサーバーに害は無い。
//
// # なぜ moov を先頭に要求するか
//
// 先頭に moov があれば、ファイルの頭（videoHeadLimit まで）を1回読むだけで全部の表が
// 手に入る。末尾にあると、25MB を読むか、範囲を指定してもう1往復する必要がある。
// 再生側にとっても、moov が先頭にあれば全体を取る前に再生を始められる。
// クライアントは必ず moov を先頭に置いて書き出すので（SPACE-client の lib/video）、
// ここで先頭を要求しても正しく作られた投稿は落ちない。
//
// # なぜ自前で読むか
//
// 必要なのは決まった十数種類の箱の、決まった欄だけ。汎用の MP4 ライブラリを入れると
// 依存が増えるわりに、どの欄を信じているかが読み取りにくくなる。ここは「どの欄を
// 見て何を拒否しているか」がそのまま読めることを優先した。

// VideoInfo は受け入れた動画の実測値。クライアントの申告ではなく、ファイルの中身から読む。
// SizeBytes は InspectMP4 では埋めない（受け入れ側が StatObject で測った値を入れる）。
type VideoInfo = model.VideoProbe

const (
	// VideoMaxBytes は圧縮後の動画の上限。仕様では 25MB ＝ 25,000,000 バイト
	// （MiB ではない）。他の種別の上限は MiB で書いてあるので、混ぜないよう分けてある。
	VideoMaxBytes int64 = 25_000_000
	// videoMaxDurationMs は長さの上限。60秒に 0.1 秒の幅を足してある。
	// 再エンコードすると、AAC のエンコーダーが先頭に無音を足すぶん 60.02 秒程度に
	// なることがあり、60.000 秒ちょうどで切ると元が 60 秒の動画が通らない。
	videoMaxDurationMs = 60_100
	// 横向きなら 1280×720、縦向きなら 720×1280 に収まること。
	videoMaxLongSide  = 1280
	videoMaxShortSide = 720
	// videoMaxFPSMilli はフレームレートの上限。30fps に、タイムスケールの丸めで
	// 出る誤差のぶんだけ幅を足してある（29.97fps や 30.000x fps を落とさないため）。
	videoMaxFPSMilli = 30_050
	// videoHeadLimit は先頭から読むバイト数。60秒の動画の moov は数十KBに収まる。
	// これを超える moov は、作り方がおかしいか、表を水増しした入力なので受け付けない。
	videoHeadLimit = 1 << 20
	// maxSamplesPerTrack は1トラックの最大サンプル数。60秒・30fps の映像なら約1800、
	// 48kHz の AAC でも約2800。表の水増しで検査の CPU を食わせないための上限。
	maxSamplesPerTrack = 20_000
	// audioPrimingSlackMs は音声の素の長さに許す余り（inspectMoov 参照）。
	audioPrimingSlackMs = 1_000
)

// errVideo は利用者に返す文言付きのエラーを作る。
func errVideo(msg string) error { return apperr.InvalidInput(msg) }

var (
	errVideoBroken     = errVideo("動画ファイルが壊れているか、対応していない形式です")
	errVideoNotMP4     = errVideo("動画は MP4 形式にしてください")
	errVideoNotFast    = errVideo("動画の形式に対応していません（再生情報がファイルの先頭にありません）")
	errVideoCodec      = errVideo("動画の映像は H.264 にしてください")
	errVideoAudioCodec = errVideo("動画の音声は AAC にしてください")
	errVideoTracks     = errVideo("動画に対応していないトラックが含まれています")
	errVideoTooLong    = errVideo("動画は60秒以内にしてください")
	errVideoTooLarge   = errVideo("動画の解像度は 720p（1280×720）以内にしてください")
	errVideoFPS        = errVideo("動画のフレームレートは 30fps 以内にしてください")
)

// InspectMP4 は動画ファイルの先頭 head（ファイル全体の大きさは fileSize）を読み、
// 受け付けてよい動画かを確かめて実測値を返す。
func InspectMP4(head []byte, fileSize int64) (VideoInfo, error) {
	if fileSize <= 0 || int64(len(head)) > fileSize {
		return VideoInfo{}, errVideoBroken
	}
	var (
		off       int64
		sawFtyp   bool
		moov      []byte
		mdatStart int64 = -1
		mdatEnd   int64
	)
	for off < fileSize {
		h, err := readHeader(head, off, fileSize)
		if err != nil {
			if off >= int64(len(head)) {
				// 先頭に読んだ範囲で moov と mdat の頭まで届かなかった。
				return VideoInfo{}, errVideoNotFast
			}
			return VideoInfo{}, errVideoBroken
		}
		end := off + h.size
		switch h.typ {
		case "ftyp":
			if sawFtyp || off != 0 || end > int64(len(head)) {
				return VideoInfo{}, errVideoNotMP4
			}
			if !isMP4Brand(head[off+h.hdr : end]) {
				return VideoInfo{}, errVideoNotMP4
			}
			sawFtyp = true
		case "moov":
			if !sawFtyp || moov != nil {
				return VideoInfo{}, errVideoBroken
			}
			if end > int64(len(head)) {
				return VideoInfo{}, errVideoNotFast
			}
			moov = head[off+h.hdr : end]
		case "mdat":
			if !sawFtyp {
				return VideoInfo{}, errVideoNotMP4
			}
			if moov == nil {
				// moov が mdat の後ろにある（先頭に置かれていない）。
				return VideoInfo{}, errVideoNotFast
			}
			if mdatStart >= 0 {
				return VideoInfo{}, errVideoBroken
			}
			mdatStart, mdatEnd = off+h.hdr, end
			if mdatEnd != fileSize {
				// mdat の後ろに何かが付いている。書き出し方として想定しておらず、
				// 何が付いているかも読まないと分からないので受け付けない。
				return VideoInfo{}, errVideoBroken
			}
		case "free", "skip", "wide":
			// 詰め物。中身は見ない。
		case "moof", "mfra", "sidx", "styp":
			// 断片化した MP4。表が moov に揃っておらず、ここでは検査しきれない。
			return VideoInfo{}, errVideoNotFast
		default:
			return VideoInfo{}, errVideoBroken
		}
		off = end
	}
	if !sawFtyp {
		return VideoInfo{}, errVideoNotMP4
	}
	if moov == nil || mdatStart < 0 {
		return VideoInfo{}, errVideoBroken
	}
	return inspectMoov(moov, mdatStart, mdatEnd)
}

type boxHeader struct {
	typ  string
	hdr  int64 // ヘッダーの長さ
	size int64 // ヘッダーを含む箱全体の長さ
}

// readHeader は b[off:] にある箱のヘッダーを読む。limit は親の終わり（箱がそこを
// 越えてはいけない）。b に入っている範囲より後ろのヘッダーは読めないのでエラーになる。
func readHeader(b []byte, off, limit int64) (boxHeader, error) {
	if off < 0 || off+8 > int64(len(b)) || off+8 > limit {
		return boxHeader{}, errVideoBroken
	}
	size := int64(binary.BigEndian.Uint32(b[off:]))
	typ := string(b[off+4 : off+8])
	hdr := int64(8)
	switch size {
	case 0:
		// 親の終わりまで。
		size = limit - off
	case 1:
		if off+16 > int64(len(b)) || off+16 > limit {
			return boxHeader{}, errVideoBroken
		}
		large := binary.BigEndian.Uint64(b[off+8:])
		if large > uint64(limit-off) {
			return boxHeader{}, errVideoBroken
		}
		size = int64(large)
		hdr = 16
	}
	if size < hdr || off+size > limit {
		return boxHeader{}, errVideoBroken
	}
	return boxHeader{typ: typ, hdr: hdr, size: size}, nil
}

// children は b を箱の並びとして読み、型ごとの中身を返す（同じ型は出てきた順）。
func children(b []byte) (map[string][][]byte, error) {
	out := map[string][][]byte{}
	var off int64
	n := int64(len(b))
	for off < n {
		if n-off < 8 {
			// 8バイト未満の端数は詰め物として許す（古い書き出しが末尾に 0 を置くことがある）。
			if allZero(b[off:]) {
				break
			}
			return nil, errVideoBroken
		}
		h, err := readHeader(b, off, n)
		if err != nil {
			return nil, err
		}
		out[h.typ] = append(out[h.typ], b[off+h.hdr:off+h.size])
		off += h.size
	}
	return out, nil
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// one は型 typ の箱がちょうど1つあることを確かめて中身を返す。
func one(m map[string][][]byte, typ string) ([]byte, error) {
	if len(m[typ]) != 1 {
		return nil, errVideoBroken
	}
	return m[typ][0], nil
}

// isMP4Brand は ftyp の主ブランドか互換ブランドに ISO の MP4 があるかを見る。
// QuickTime（qt  ）だけを名乗るファイル（iPhone の .mov）はここで落ちる。
func isMP4Brand(p []byte) bool {
	if len(p) < 8 {
		return false
	}
	brands := [][]byte{p[0:4]}
	for i := 8; i+4 <= len(p); i += 4 {
		brands = append(brands, p[i:i+4])
	}
	for _, b := range brands {
		switch string(b) {
		case "isom", "iso2", "iso3", "iso4", "iso5", "iso6", "mp41", "mp42", "avc1", "M4V ", "dash":
			return true
		}
	}
	return false
}

type reader struct {
	b   []byte
	off int
	err error
}

func (r *reader) need(n int) bool {
	if r.err != nil {
		return false
	}
	if n < 0 || r.off+n > len(r.b) {
		r.err = errVideoBroken
		return false
	}
	return true
}

func (r *reader) skip(n int) {
	if r.need(n) {
		r.off += n
	}
}

func (r *reader) u8() uint8 {
	if !r.need(1) {
		return 0
	}
	v := r.b[r.off]
	r.off++
	return v
}

func (r *reader) u16() uint16 {
	if !r.need(2) {
		return 0
	}
	v := binary.BigEndian.Uint16(r.b[r.off:])
	r.off += 2
	return v
}

func (r *reader) u32() uint32 {
	if !r.need(4) {
		return 0
	}
	v := binary.BigEndian.Uint32(r.b[r.off:])
	r.off += 4
	return v
}

func (r *reader) u64() uint64 {
	if !r.need(8) {
		return 0
	}
	v := binary.BigEndian.Uint64(r.b[r.off:])
	r.off += 8
	return v
}

func (r *reader) bytes(n int) []byte {
	if !r.need(n) {
		return nil
	}
	v := r.b[r.off : r.off+n]
	r.off += n
	return v
}

func (r *reader) rest() int { return len(r.b) - r.off }

// fullBoxVersion は FullBox の version を読み、flags を読み飛ばす。
func (r *reader) fullBoxVersion() uint8 {
	v := r.u8()
	r.skip(3)
	return v
}

// timeFields は mvhd / mdhd の時刻欄を読み、timescale と duration を返す。
func timeFields(p []byte) (timescale uint32, duration uint64, err error) {
	r := &reader{b: p}
	if r.fullBoxVersion() == 1 {
		r.skip(16)
		timescale = r.u32()
		duration = r.u64()
	} else {
		r.skip(8)
		timescale = r.u32()
		duration = uint64(r.u32())
	}
	if r.err != nil || timescale == 0 {
		return 0, 0, errVideoBroken
	}
	return timescale, duration, nil
}

type trackInfo struct {
	handler        string
	tkhdDuration   uint64 // movie のタイムスケール
	tkhdW, tkhdH   uint32 // 16.16 固定小数点の整数部
	mediaTimescale uint32
	mediaDuration  uint64 // mdhd と stts の大きい方
	sampleCount    int
	codec          string
	width, height  int
}

func inspectMoov(moov []byte, mdatStart, mdatEnd int64) (VideoInfo, error) {
	m, err := children(moov)
	if err != nil {
		return VideoInfo{}, err
	}
	if len(m["mvex"]) > 0 {
		return VideoInfo{}, errVideoNotFast
	}
	mvhd, err := one(m, "mvhd")
	if err != nil {
		return VideoInfo{}, err
	}
	movieTimescale, movieDuration, err := timeFields(mvhd)
	if err != nil {
		return VideoInfo{}, err
	}

	var video, audio *trackInfo
	for _, trak := range m["trak"] {
		t, err := inspectTrak(trak, mdatStart, mdatEnd)
		if err != nil {
			return VideoInfo{}, err
		}
		switch t.handler {
		case "vide":
			if video != nil {
				return VideoInfo{}, errVideoTracks
			}
			video = t
		case "soun":
			if audio != nil {
				return VideoInfo{}, errVideoTracks
			}
			audio = t
		default:
			return VideoInfo{}, errVideoTracks
		}
	}
	if video == nil {
		return VideoInfo{}, errVideoBroken
	}

	// 長さは、ムービー全体・各トラックのどれで測っても上限内であること。
	// どれか1つだけを見ると、残りを水増しした入力（映像は短く、音声だけ10分など）が通る。
	//
	// 音声の素の長さ（mdhd / stts）だけは少し長くてよい。AAC は先頭に1〜2フレームぶんの
	// 準備用サンプルを持ち、それを edit list で切って再生する。素の長さで測ると、60秒
	// ちょうどの動画が 60.1 秒を超えて落ちる（8kHz なら 1024 サンプルで 0.128 秒）。
	durMs := max(toMs(movieDuration, movieTimescale), toMs(video.tkhdDuration, movieTimescale), toMs(video.mediaDuration, video.mediaTimescale))
	if audio != nil {
		durMs = max(durMs, toMs(audio.tkhdDuration, movieTimescale))
		if toMs(audio.mediaDuration, audio.mediaTimescale) > videoMaxDurationMs+audioPrimingSlackMs {
			return VideoInfo{}, errVideoTooLong
		}
	}
	if durMs > videoMaxDurationMs {
		return VideoInfo{}, errVideoTooLong
	}

	// 解像度は符号化された大きさ（avc1）と表示の大きさ（tkhd）の両方を見る。
	// 表示側だけ小さく名乗って、中身は 4K という入力を通さないため。
	if !withinVideoBox(video.width, video.height) {
		if video.width <= 0 || video.height <= 0 {
			return VideoInfo{}, errVideoBroken
		}
		return VideoInfo{}, errVideoTooLarge
	}
	// tkhd の寸法を 0 で書く書き出しがあるので、0 のときは符号化側の寸法で足りる。
	if video.tkhdW != 0 || video.tkhdH != 0 {
		if !withinVideoBox(int(video.tkhdW), int(video.tkhdH)) {
			return VideoInfo{}, errVideoTooLarge
		}
	}

	if video.mediaDuration == 0 || video.sampleCount == 0 {
		return VideoInfo{}, errVideoBroken
	}
	fpsMilli := int64(video.sampleCount) * 1000 * int64(video.mediaTimescale) / int64(video.mediaDuration)
	if fpsMilli > videoMaxFPSMilli {
		return VideoInfo{}, errVideoFPS
	}

	return VideoInfo{
		DurationMs: durMs,
		Width:      video.width,
		Height:     video.height,
		FPSMilli:   fpsMilli,
		HasAudio:   audio != nil,
	}, nil
}

// withinVideoBox は縦横が 1280×720（縦向きなら 720×1280）に収まるかを返す。
func withinVideoBox(w, h int) bool {
	return w > 0 && h > 0 && max(w, h) <= videoMaxLongSide && min(w, h) <= videoMaxShortSide
}

func toMs(duration uint64, timescale uint32) int64 {
	if timescale == 0 {
		return 0
	}
	// 60秒程度の値しか扱わないが、水増しされた duration で桁あふれしないよう割ってから掛ける。
	sec := duration / uint64(timescale)
	rem := duration % uint64(timescale)
	if sec > 1<<40 {
		return 1 << 62
	}
	return int64(sec*1000 + rem*1000/uint64(timescale))
}

func inspectTrak(trak []byte, mdatStart, mdatEnd int64) (*trackInfo, error) {
	m, err := children(trak)
	if err != nil {
		return nil, err
	}
	tkhd, err := one(m, "tkhd")
	if err != nil {
		return nil, err
	}
	t := &trackInfo{}
	r := &reader{b: tkhd}
	if r.fullBoxVersion() == 1 {
		r.skip(16 + 4 + 4)
		t.tkhdDuration = r.u64()
	} else {
		r.skip(8 + 4 + 4)
		t.tkhdDuration = uint64(r.u32())
	}
	r.skip(8 + 2 + 2 + 2 + 2 + 36)
	t.tkhdW = r.u32() >> 16
	t.tkhdH = r.u32() >> 16
	if r.err != nil {
		return nil, errVideoBroken
	}

	mdiaBox, err := one(m, "mdia")
	if err != nil {
		return nil, err
	}
	mdia, err := children(mdiaBox)
	if err != nil {
		return nil, err
	}
	mdhd, err := one(mdia, "mdhd")
	if err != nil {
		return nil, err
	}
	var mdhdDuration uint64
	t.mediaTimescale, mdhdDuration, err = timeFields(mdhd)
	if err != nil {
		return nil, err
	}
	hdlr, err := one(mdia, "hdlr")
	if err != nil {
		return nil, err
	}
	hr := &reader{b: hdlr}
	hr.fullBoxVersion()
	hr.skip(4)
	t.handler = string(hr.bytes(4))
	if hr.err != nil {
		return nil, errVideoBroken
	}
	if t.handler != "vide" && t.handler != "soun" {
		return nil, errVideoTracks
	}

	minfBox, err := one(mdia, "minf")
	if err != nil {
		return nil, err
	}
	minf, err := children(minfBox)
	if err != nil {
		return nil, err
	}
	stblBox, err := one(minf, "stbl")
	if err != nil {
		return nil, err
	}
	stbl, err := children(stblBox)
	if err != nil {
		return nil, err
	}

	stsd, err := one(stbl, "stsd")
	if err != nil {
		return nil, err
	}
	if err := t.inspectSampleEntry(stsd); err != nil {
		return nil, err
	}

	sttsTotal, sttsCount, err := readStts(stbl)
	if err != nil {
		return nil, err
	}
	t.mediaDuration = max(mdhdDuration, sttsTotal)

	sizes, err := readStsz(stbl)
	if err != nil {
		return nil, err
	}
	if len(sizes) != sttsCount {
		return nil, errVideoBroken
	}
	t.sampleCount = len(sizes)
	if err := checkSampleLayout(stbl, sizes, mdatStart, mdatEnd); err != nil {
		return nil, err
	}
	return t, nil
}

// inspectSampleEntry は stsd のサンプル記述を読み、コーデックを確かめる。
func (t *trackInfo) inspectSampleEntry(stsd []byte) error {
	r := &reader{b: stsd}
	r.fullBoxVersion()
	if r.u32() != 1 {
		// 途中でコーデックが変わるトラックは扱わない。
		return errVideoBroken
	}
	if r.err != nil {
		return errVideoBroken
	}
	entries, err := children(stsd[r.off:])
	if err != nil {
		return err
	}
	if len(entries) != 1 {
		return errVideoBroken
	}
	for typ, list := range entries {
		t.codec = typ
		entry := list[0]
		switch t.handler {
		case "vide":
			if typ != "avc1" && typ != "avc3" {
				return errVideoCodec
			}
			return t.inspectAVC(entry)
		case "soun":
			if typ != "mp4a" {
				return errVideoAudioCodec
			}
			return inspectMP4A(entry)
		}
	}
	return errVideoBroken
}

// visualSampleEntryLen は VisualSampleEntry の固定部分（SampleEntry の8バイトを含む）。
const visualSampleEntryLen = 78

func (t *trackInfo) inspectAVC(entry []byte) error {
	r := &reader{b: entry}
	r.skip(8 + 16)
	t.width = int(r.u16())
	t.height = int(r.u16())
	if r.err != nil || len(entry) < visualSampleEntryLen {
		return errVideoBroken
	}
	ext, err := children(entry[visualSampleEntryLen:])
	if err != nil {
		return err
	}
	avcC, err := one(ext, "avcC")
	if err != nil {
		return err
	}
	return checkAVCC(avcC)
}

// checkAVCC は H.264 の設定（SPS と PPS）が読める形になっているかを見る。
// 設定が欠けた H.264 はどのブラウザーでも再生できない。
func checkAVCC(p []byte) error {
	r := &reader{b: p}
	if r.u8() != 1 {
		return errVideoBroken
	}
	r.skip(3) // profile, compatibility, level
	if r.u8()&0x03 == 2 {
		// NAL の長さ欄が3バイト、は規格に無い。
		return errVideoBroken
	}
	numSPS := int(r.u8() & 0x1f)
	if numSPS == 0 {
		return errVideoBroken
	}
	for range numSPS {
		nal := r.bytes(int(r.u16()))
		if r.err != nil || len(nal) == 0 || nal[0]&0x1f != 7 {
			return errVideoBroken
		}
	}
	numPPS := int(r.u8())
	if numPPS == 0 {
		return errVideoBroken
	}
	for range numPPS {
		nal := r.bytes(int(r.u16()))
		if r.err != nil || len(nal) == 0 || nal[0]&0x1f != 8 {
			return errVideoBroken
		}
	}
	return nil
}

// audioSampleEntryLen は AudioSampleEntry（version 0）の固定部分。
const audioSampleEntryLen = 28

func inspectMP4A(entry []byte) error {
	if len(entry) < audioSampleEntryLen {
		return errVideoBroken
	}
	r := &reader{b: entry}
	r.skip(8)
	if r.u16() != 0 {
		// QuickTime の音声記述（version 1/2）。MP4 では使わない。
		return errVideoAudioCodec
	}
	ext, err := children(entry[audioSampleEntryLen:])
	if err != nil {
		return err
	}
	esds, err := one(ext, "esds")
	if err != nil {
		return errVideoAudioCodec
	}
	return checkESDS(esds)
}

// checkESDS は音声が AAC であることを確かめる。
func checkESDS(p []byte) error {
	r := &reader{b: p}
	r.fullBoxVersion()
	if r.u8() != 0x03 { // ES_Descriptor
		return errVideoAudioCodec
	}
	readDescLen(r)
	r.skip(2) // ES_ID
	flags := r.u8()
	if flags&0x80 != 0 {
		r.skip(2)
	}
	if flags&0x40 != 0 {
		r.skip(int(r.u8()))
	}
	if flags&0x20 != 0 {
		r.skip(2)
	}
	if r.u8() != 0x04 { // DecoderConfigDescriptor
		return errVideoAudioCodec
	}
	readDescLen(r)
	oti := r.u8()
	r.skip(12) // streamType 等, bufferSize, maxBitrate, avgBitrate
	if r.err != nil {
		return errVideoBroken
	}
	switch oti {
	case 0x66, 0x67, 0x68: // MPEG-2 AAC（Main / LC / SSR）
		return nil
	case 0x40: // MPEG-4 Audio。中身が AAC かは AudioSpecificConfig の先頭で決まる。
	default:
		return errVideoAudioCodec
	}
	if r.u8() != 0x05 { // DecoderSpecificInfo
		return errVideoAudioCodec
	}
	n := readDescLen(r)
	asc := r.bytes(n)
	if r.err != nil || len(asc) == 0 {
		return errVideoBroken
	}
	switch asc[0] >> 3 {
	case 1, 2, 5, 29: // AAC Main / LC / HE-AAC / HE-AAC v2
		return nil
	}
	return errVideoAudioCodec
}

// readDescLen は記述子の可変長の長さ欄（最大4バイト）を読む。
func readDescLen(r *reader) int {
	n := 0
	for range 4 {
		b := r.u8()
		n = n<<7 | int(b&0x7f)
		if b&0x80 == 0 {
			break
		}
	}
	return n
}

// readStts はサンプルの時間表を読み、合計の長さとサンプル数を返す。
func readStts(stbl map[string][][]byte) (total uint64, count int, err error) {
	p, err := one(stbl, "stts")
	if err != nil {
		return 0, 0, err
	}
	r := &reader{b: p}
	r.fullBoxVersion()
	n := int(r.u32())
	if r.err != nil || n < 0 || n > r.rest()/8 {
		return 0, 0, errVideoBroken
	}
	for range n {
		c := int(r.u32())
		d := uint64(r.u32())
		count += c
		if count > maxSamplesPerTrack {
			return 0, 0, errVideoBroken
		}
		total += uint64(c) * d
	}
	return total, count, r.err
}

// readStsz はサンプルごとの大きさを読む。
func readStsz(stbl map[string][][]byte) ([]uint32, error) {
	if len(stbl["stz2"]) > 0 {
		return nil, errVideoBroken
	}
	p, err := one(stbl, "stsz")
	if err != nil {
		return nil, err
	}
	r := &reader{b: p}
	r.fullBoxVersion()
	fixed := r.u32()
	n := int(r.u32())
	if r.err != nil || n < 0 || n > maxSamplesPerTrack {
		return nil, errVideoBroken
	}
	sizes := make([]uint32, n)
	if fixed != 0 {
		for i := range sizes {
			sizes[i] = fixed
		}
		return sizes, nil
	}
	if n > r.rest()/4 {
		return nil, errVideoBroken
	}
	for i := range sizes {
		sizes[i] = r.u32()
	}
	return sizes, r.err
}

// checkSampleLayout は、全サンプルが mdat の中身の範囲を指していることと、
// チャンクの表がサンプル数とちょうど噛み合うことを確かめる。
//
// 表の途中で範囲外を指すファイルは、再生側が範囲外を読もうとして止まる（壊れた
// ファイルの典型）。表どうしが噛み合わない場合も同じ。
func checkSampleLayout(stbl map[string][][]byte, sizes []uint32, mdatStart, mdatEnd int64) error {
	offsets, err := readChunkOffsets(stbl)
	if err != nil {
		return err
	}
	stsc, err := one(stbl, "stsc")
	if err != nil {
		return err
	}
	r := &reader{b: stsc}
	r.fullBoxVersion()
	n := int(r.u32())
	if r.err != nil || n < 0 || n > r.rest()/12 {
		return errVideoBroken
	}
	type run struct{ first, perChunk uint32 }
	runs := make([]run, n)
	for i := range runs {
		runs[i] = run{first: r.u32(), perChunk: r.u32()}
		r.skip(4)
		if i == 0 && runs[i].first != 1 {
			return errVideoBroken
		}
		if i > 0 && runs[i].first <= runs[i-1].first {
			return errVideoBroken
		}
	}
	if r.err != nil {
		return errVideoBroken
	}
	if len(sizes) > 0 && (len(runs) == 0 || len(offsets) == 0) {
		return errVideoBroken
	}

	sample := 0
	ri := 0
	for chunk := range offsets {
		chunkNo := uint32(chunk + 1)
		for ri+1 < len(runs) && runs[ri+1].first <= chunkNo {
			ri++
		}
		if len(runs) == 0 {
			break
		}
		pos := int64(offsets[chunk])
		for range runs[ri].perChunk {
			if sample >= len(sizes) {
				return errVideoBroken
			}
			end := pos + int64(sizes[sample])
			if pos < mdatStart || end > mdatEnd {
				return errVideoBroken
			}
			pos = end
			sample++
		}
	}
	if sample != len(sizes) {
		return errVideoBroken
	}
	return nil
}

func readChunkOffsets(stbl map[string][][]byte) ([]uint64, error) {
	var p []byte
	wide := false
	switch {
	case len(stbl["stco"]) == 1 && len(stbl["co64"]) == 0:
		p = stbl["stco"][0]
	case len(stbl["co64"]) == 1 && len(stbl["stco"]) == 0:
		p = stbl["co64"][0]
		wide = true
	default:
		return nil, errVideoBroken
	}
	r := &reader{b: p}
	r.fullBoxVersion()
	n := int(r.u32())
	width := 4
	if wide {
		width = 8
	}
	if r.err != nil || n < 0 || n > maxSamplesPerTrack || n > r.rest()/width {
		return nil, errVideoBroken
	}
	offsets := make([]uint64, n)
	for i := range offsets {
		if wide {
			offsets[i] = r.u64()
		} else {
			offsets[i] = uint64(r.u32())
		}
	}
	return offsets, r.err
}

// IsVideoRejection は err が「動画が規格外」によるもの（利用者に理由を見せてよい）かを返す。
func IsVideoRejection(err error) bool {
	var ae *apperr.Error
	return errors.As(err, &ae) && ae.Code == apperr.CodeInvalidInput
}

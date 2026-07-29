package media

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// HLSThreshold is the duration in seconds above which a video will be
// segmented into HLS (m3u8 + ts). Videos <= this threshold are left as-is.
const HLSThreshold = 30.0

// IsVideoExt reports whether the lowercased file extension is a recognised
// video container.
func IsVideoExt(ext string) bool {
	switch ext {
	case ".mp4", ".webm", ".ogg", ".mov":
		return true
	}
	return false
}

// ffprobeFormat is the minimal JSON structure we need from ffprobe.
type ffprobeFormat struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// GetVideoDuration returns the duration of a video file in seconds.
// It shells out to ffprobe.  An error is returned when ffprobe is not
// installed or cannot parse the file.
func GetVideoDuration(filePath string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		filePath,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed (is ffmpeg installed?): %w", err)
	}

	var info ffprobeFormat
	if err := json.Unmarshal(out, &info); err != nil {
		return 0, fmt.Errorf("parse ffprobe output: %w", err)
	}

	var duration float64
	if _, err := fmt.Sscanf(info.Format.Duration, "%f", &duration); err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", info.Format.Duration, err)
	}
	return duration, nil
}

// SegmentVideo runs ffmpeg to create an HLS playlist + TS segments from the
// input video.  It uses stream copy (-c copy) so there is no quality loss
// and the operation is fast.
//
// Parameters:
//   - inputPath:  path to the source video file
//   - outputDir:  directory where .m3u8 and .ts files will be written
//   - baseName:   output filename stem (without extension), e.g. "uuid"
//
// Returns the absolute path to the .m3u8 file and a list of absolute paths
// to the generated .ts segment files.
func SegmentVideo(inputPath, outputDir, baseName string) (string, []string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", nil, fmt.Errorf("create output dir: %w", err)
	}

	m3u8Name := baseName + ".m3u8"
	tsPattern := baseName + "_%03d.ts"
	m3u8Path := filepath.Join(outputDir, m3u8Name)
	tsPatternFull := filepath.Join(outputDir, tsPattern)

	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-c", "copy",
		"-hls_time", "10",
		"-hls_list_size", "0",
		"-hls_segment_filename", tsPatternFull,
		m3u8Path,
	)
	// Capture stderr for diagnostics; ffmpeg writes progress to stderr.
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", nil, fmt.Errorf("ffmpeg segmentation failed: %w\n%s", err, string(out))
	}

	// Collect the generated TS files from the output directory.
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return m3u8Path, nil, fmt.Errorf("read output dir: %w", err)
	}

	prefix := baseName + "_"
	var tsFiles []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".ts") {
			tsFiles = append(tsFiles, filepath.Join(outputDir, name))
		}
	}

	return m3u8Path, tsFiles, nil
}

// GenerateVideoPoster extracts a single frame from a video file and saves it as
// a JPEG image. It uses ffmpeg to seek to 1 second (to skip black intro frames)
// and capture one frame at medium quality.
//
// Parameters:
//   - inputPath:  path to the source video file
//   - outputPath: path where the poster JPEG will be written
func GenerateVideoPoster(inputPath, outputPath string) error {
	cmd := exec.Command("ffmpeg",
		"-ss", "1", // seek to 1 second to skip black frames
		"-i", inputPath,
		"-vframes", "1",
		"-q:v", "2",
		"-y", // overwrite output
		outputPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg poster generation failed: %w\n%s", err, string(out))
	}
	return nil
}

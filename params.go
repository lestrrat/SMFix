package main

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

type slicerParams struct {
	Version            int    // 0 or 1
	Model              string // A250/350/400/J1
	ToolHead           string // ;tool_head
	LeftExtruderUsed   bool
	RightExtruderUsed  bool
	PrintMode          string
	PrinterModel       string // ;machine
	PrinterNotes       string
	LayerHeight        float64
	TotalLayers        int
	TotalLines         int // without headers
	EstimatedTimeSec   int // time * 1.07
	NozzleTemperatures []float64
	NozzleDiameters    []float64
	Retractions        []float64
	SwitchRetraction   float64
	BedTemperatures    []float64
	FilamentTypes      []string
	FilamentUsed       []float64 // mm
	FilamentUsedWeight []float64 // len * 1.24g/cm3 * pi * 1.75/2 * 1.75/2
	PrintSpeedSec      float64   // ;work_speed
	MinX               float64
	MinY               float64
	MinZ               float64
	MaxX               float64
	MaxY               float64
	MaxZ               float64
	Thumbnail          []byte
}

var Params = slicerParams{
	Version:            0,
	Model:              "",
	ToolHead:           ToolheadSingle,
	PrintMode:          PrintModeDefault,
	LeftExtruderUsed:   false,
	RightExtruderUsed:  false,
	PrinterModel:       "",
	PrinterNotes:       "",
	LayerHeight:        0,
	TotalLayers:        0,
	TotalLines:         0,
	EstimatedTimeSec:   0,
	NozzleTemperatures: []float64{-1, -1},
	NozzleDiameters:    []float64{-1, -1},
	Retractions:        []float64{-1, -1},
	SwitchRetraction:   0,
	BedTemperatures:    []float64{-1, -1},
	FilamentTypes:      []string{"", ""},
	FilamentUsed:       []float64{-1, -1},
	FilamentUsedWeight: []float64{-1, -1},
	PrintSpeedSec:      0,
	MinX:               0,
	MinY:               0,
	MinZ:               0,
	MaxX:               0,
	MaxY:               0,
	MaxZ:               0,
	Thumbnail:          []byte{},
}

func (p *slicerParams) EffectiveNozzleTemperature() float64 {
	return p.effective(p.NozzleTemperatures[0], p.NozzleTemperatures[1])
}

func (p *slicerParams) EffectiveBedTemperature() float64 {
	return p.effective(p.BedTemperatures[0], p.BedTemperatures[1])
}

func (p *slicerParams) AllFilamentUsed() float64 {
	return p.FilamentUsed[0] + p.FilamentUsed[1]
}

func (p *slicerParams) AllFilamentUsedWeight() float64 {
	return p.FilamentUsedWeight[0] + p.FilamentUsedWeight[1]
}

func (p *slicerParams) effective(x, y float64) float64 {
	if x < 1 {
		return y
	}
	return x
}

func parseParams(f io.Reader) error {
	sc := bufio.NewScanner(f)

	var (
		thumbnail_bytes [][]byte
		thumbnail_start = false

		model              string
		bed_shape          string
		printers_condition string

		retract_len          = []float64{-1, -1}
		filament_retract_len = []float64{-1, -1}
	)

	//////// scan
	for sc.Scan() {
		Params.TotalLines++

		line := strings.TrimSpace(sc.Text())
		if len(line) < 1 {
			continue
		}

		if strings.HasPrefix(line, "; Postprocessed by smfix") {
			return errors.New("No need to fix again.")
		} else if strings.HasPrefix(line, "; SNAPMAKER_GCODE_V1") {
			Params.Version = 1
		} else if strings.HasPrefix(line, "M605 S2") {
			Params.PrintMode = PrintModeDuplication
		} else if strings.HasPrefix(line, "M605 S3") {
			Params.PrintMode = PrintModeMirror
		} else if strings.HasPrefix(line, "M605 S4") {
			Params.PrintMode = PrintModeBackup
		} else if strings.HasPrefix(line, "; thumbnail begin ") {
			thumbnail_start = true
		} else if strings.HasPrefix(line, "; thumbnail end") {
			thumbnail_bytes = append(thumbnail_bytes, []byte(line))
			thumbnail_start = false
		} else if v, ok := getSetting(line, "filament used [mm]"); ok {
			Params.FilamentUsed = splitFloat(v)
		} else if v, ok := getSetting(line, "filament used [g]"); ok {
			Params.FilamentUsedWeight = splitFloat(v)
		} else if v, ok := getSetting(line, "estimated printing time (normal mode)"); ok {
			Params.EstimatedTimeSec = convertEstimatedTime(v)
		} else if v, ok := getSetting(line, "filament_type"); ok {
			Params.FilamentTypes = split(v)
		} else if v, ok := getSetting(line, "total_layer_number"); ok {
			Params.TotalLayers = parseInt(v)
		} else if v, ok := getSetting(line, "filament_retract_length", "filament_retraction_length" /*bbs*/); ok {
			filament_retract_len = splitFloat(v)
		} else if v, ok := getSetting(line, "retract_length", "retraction_length" /*bbs*/); ok {
			retract_len = splitFloat(v)
		} else if v, ok := getSetting(line, "retract_length_toolchange"); ok {
			Params.SwitchRetraction = parseFloat(v)
		} else if v, ok := getSetting(line, "nozzle_diameter"); ok {
			Params.NozzleDiameters = splitFloat(v)
		} else if v, ok := getSetting(line, "layer_height", "first_layer_height"); ok && Params.LayerHeight == 0 {
			Params.LayerHeight = parseFloat(v)
		} else if v, ok := getSetting(line, "printer_notes"); ok {
			Params.PrinterNotes = v
		} else if v, ok := getSetting(line, "max_print_speed", "outer_wall_speed" /*bbs*/); ok && Params.PrintSpeedSec == 0 {
			Params.PrintSpeedSec = parseFloat(v)
		} else if v, ok := getSetting(line, "first_layer_temperature", "temperature", "nozzle_temperature_initial_layer", "nozzle_temperature" /*bbs*/); ok && Params.NozzleTemperatures[0] == -1 {
			Params.NozzleTemperatures = splitFloat(v)
		} else if v, ok := getSetting(line, "first_layer_bed_temperature", "bed_temperature", "hot_plate_temp_initial_layer", "hot_plate_temp" /*bbs*/); ok && Params.BedTemperatures[0] == -1 {
			Params.BedTemperatures = splitFloat(v)
		} else if v, ok := getSetting(line, "min_x"); ok {
			Params.MinX = parseFloat(v)
		} else if v, ok := getSetting(line, "min_y"); ok {
			Params.MinY = parseFloat(v)
		} else if v, ok := getSetting(line, "min_z"); ok {
			Params.MinZ = parseFloat(v)
		} else if v, ok := getSetting(line, "max_x"); ok {
			Params.MaxX = parseFloat(v)
		} else if v, ok := getSetting(line, "max_y"); ok {
			Params.MaxY = parseFloat(v)
		} else if v, ok := getSetting(line, "max_z"); ok {
			Params.MaxZ = parseFloat(v)
		} else if v, ok := getSetting(line, "printer_model"); ok {
			model = v
		} else if v, ok := getSetting(line, "bed_shape"); ok {
			bed_shape = v
		} else if v, ok := getSetting(line, "compatible_printers_condition_cummulative", "print_compatible_printers" /*bbs*/); ok {
			printers_condition = v
		}

		if thumbnail_start {
			thumbnail_bytes = append(thumbnail_bytes, []byte(line))
		}
	}

	if err := sc.Err(); err != nil {
		return err
	}

	//////// process params
	if len(thumbnail_bytes) > 0 {
		Params.Thumbnail = convertThumbnail(thumbnail_bytes)
	}

	Params.Retractions = retract_len
	// use filament_retract_len overwrite retract_len
	if filament_retract_len[0] > 0 {
		Params.Retractions[0] = filament_retract_len[0]
	}
	if filament_retract_len[1] > 0 {
		Params.Retractions[1] = filament_retract_len[1]
	}

	if Params.FilamentUsed[0] > 0 {
		Params.LeftExtruderUsed = true
	} else {
		// reset T0
		Params.FilamentTypes[0] = "-"
		Params.NozzleTemperatures[0] = 0
		Params.BedTemperatures[0] = -1
		Params.Retractions[0] = 0
	}

	if Params.FilamentUsed[1] > 0 {
		Params.RightExtruderUsed = true
	} else {
		// reset T1
		Params.FilamentTypes[1] = "-"
		Params.NozzleTemperatures[1] = 0
		Params.BedTemperatures[1] = -1
		Params.Retractions[1] = 0
	}

	if Params.LeftExtruderUsed && Params.RightExtruderUsed {
		Params.ToolHead = ToolheadDual
	}

	if Params.PrintMode == PrintModeMirror || Params.PrintMode == PrintModeDuplication {
		// is IDEX
		Params.Version = 1
		Params.Model = ModelJ1
	}

	// overwrite slicer version
	if strings.Contains(Params.PrinterNotes, "SNAPMAKER_GCODE_V1") {
		Params.Version = 1
	} else if strings.Contains(Params.PrinterNotes, "SNAPMAKER_GCODE_V0") {
		Params.Version = 0
	}

	{
		// printer model && slicer version
		var models = map[string]string{
			"A150":    ModelA150,
			"160x160": ModelA150,

			"A250":    ModelA250,
			"230x250": ModelA250,

			"A350":    ModelA350,
			"320x350": ModelA350,

			"A400":    ModelA400,
			"Artisan": ModelA400,
			"400x400": ModelA400,

			"J1":      ModelJ1,
			"312x200": ModelJ1,
			"324x200": ModelJ1,
			"300x200": ModelJ1,
		}
		for k, v := range models {
			if strings.Contains(model, k) {
				Params.Model = v
				break
			}
			if strings.Contains(printers_condition, k) {
				Params.Model = v
				break
			}
			if strings.Contains(bed_shape, k) {
				Params.Model = v
				break
			}
		}
		if Params.Model == ModelJ1 {
			// but J1 only support v1
			Params.Version = 1
		}
	}

	return nil
}

package chute

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
)

type VolumeInspection struct {
	Volume     string
	PVC        string
	Namespace  string
	State      string
	Robustness string
	Replicas   int
	Pods       int
}

func InspectVolumes(index *Index) []VolumeInspection {
	var rows []VolumeInspection

	for _, volume := range index.LonghornVolumes() {
		projection, err := ProjectVolume(index, volume.Name)
		if err != nil {
			continue
		}

		row := VolumeInspection{
			Volume:     volume.Name,
			State:      displayString(nestedString(volume.Data, "status", "state")),
			Robustness: displayString(nestedString(volume.Data, "status", "robustness")),
			Replicas:   len(projection.Replicas),
			Pods:       len(projection.Pods),
			PVC:        "-",
			Namespace:  "-",
		}
		if projection.PVC != nil {
			row.PVC = projection.PVC.Name
			row.Namespace = projection.PVC.Namespace
		}
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Namespace == rows[j].Namespace {
			return rows[i].Volume < rows[j].Volume
		}
		return rows[i].Namespace < rows[j].Namespace
	})

	return rows
}

func WriteInspection(writer io.Writer, rows []VolumeInspection) error {
	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "VOLUME\tPVC\tNAMESPACE\tSTATE\tROBUSTNESS\tREPLICAS\tPODS"); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(
			table,
			"%s\t%s\t%s\t%s\t%s\t%d\t%d\n",
			row.Volume,
			row.PVC,
			row.Namespace,
			row.State,
			row.Robustness,
			row.Replicas,
			row.Pods,
		); err != nil {
			return err
		}
	}
	return table.Flush()
}

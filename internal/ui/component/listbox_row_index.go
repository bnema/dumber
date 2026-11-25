package component

import "bnema.dev/puregotk/v4/gtk"

// listBoxRowIndex returns the row's index, or -1 when the row is not in a
// list box. gtk_list_box_row_get_index returns a C int, but the binding reads
// it as a 64-bit Go int without sign extension, so -1 arrives as 4294967295.
func listBoxRowIndex(row *gtk.ListBoxRow) int {
	if row == nil {
		return -1
	}
	return signExtendCInt(row.GetIndex())
}

// signExtendCInt restores the sign of a C int that the binding returned in
// a 64-bit Go int.
func signExtendCInt(v int) int {
	return int(int32(v))
}

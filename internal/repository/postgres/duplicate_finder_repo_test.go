package postgres

import "testing"

func TestIsDocumentSummarySchemaError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "missing table",
			err:  errString(`duplicate_finder_repo.FindDuplicates: ERROR: relation "document_summaries" does not exist (SQLSTATE 42P01)`),
			want: true,
		},
		{
			name: "missing column",
			err:  errString(`duplicate_finder_repo.FindDuplicates: ERROR: column ds.invoice_date does not exist (SQLSTATE 42703)`),
			want: true,
		},
		{
			name: "permission denied on summaries",
			err:  errString(`duplicate_finder_repo.FindDuplicates: ERROR: permission denied for table document_summaries (SQLSTATE 42501)`),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errString(`duplicate_finder_repo.FindDuplicates: context deadline exceeded`),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDocumentSummarySchemaError(tt.err)
			if got != tt.want {
				t.Fatalf("isDocumentSummarySchemaError() = %v, want %v", got, tt.want)
			}
		})
	}
}

type errString string

func (e errString) Error() string { return string(e) }

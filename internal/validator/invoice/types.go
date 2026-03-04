package invoice

// GSTInvoice is the strongly-typed representation of a parsed GST invoice.
type GSTInvoice struct {
	Invoice   InvoiceHeader `json:"invoice"`
	Seller    Party         `json:"seller"`
	Buyer     Party         `json:"buyer"`
	LineItems []LineItem    `json:"line_items"`
	Totals    Totals        `json:"totals"`
	Payment   Payment       `json:"payment"`
	Notes     string        `json:"notes"`
}

// InvoiceHeader holds top-level invoice metadata.
type InvoiceHeader struct {
	InvoiceNumber string `json:"invoice_number"`
	InvoiceDate   string `json:"invoice_date"`
	DueDate       string `json:"due_date"`
	InvoiceType   string `json:"invoice_type"`
	Currency      string `json:"currency"`
	PlaceOfSupply         string `json:"place_of_supply"`
	ReverseCharge         bool   `json:"reverse_charge"`
	IRN                   string `json:"irn"`
	AcknowledgementNumber string `json:"acknowledgement_number"`
	AcknowledgementDate   string `json:"acknowledgement_date"`
	QRCodeData            string `json:"qr_code_data"`
}

// Party represents a seller or buyer.
type Party struct {
	Name      string `json:"name"`
	Address   string `json:"address"`
	GSTIN     string `json:"gstin"`
	PAN       string `json:"pan"`
	State     string `json:"state"`
	StateCode string `json:"state_code"`
}

// LineItem represents a single line item on the invoice.
type LineItem struct {
	Description   string  `json:"description"`
	HSNSACCode    string  `json:"hsn_sac_code"`
	Quantity      float64 `json:"quantity"`
	Unit          string  `json:"unit"`
	UnitPrice     float64 `json:"unit_price"`
	Discount      float64 `json:"discount"`
	TaxableAmount float64 `json:"taxable_amount"`
	CGSTRate      float64 `json:"cgst_rate"`
	CGSTAmount    float64 `json:"cgst_amount"`
	SGSTRate      float64 `json:"sgst_rate"`
	SGSTAmount    float64 `json:"sgst_amount"`
	IGSTRate      float64 `json:"igst_rate"`
	IGSTAmount    float64 `json:"igst_amount"`
	Total         float64 `json:"total"`
}

// Totals holds the invoice totals.
type Totals struct {
	Subtotal      float64 `json:"subtotal"`
	TotalDiscount float64 `json:"total_discount"`
	TaxableAmount float64 `json:"taxable_amount"`
	CGST          float64 `json:"cgst"`
	SGST          float64 `json:"sgst"`
	IGST          float64 `json:"igst"`
	Cess          float64 `json:"cess"`
	RoundOff      float64 `json:"round_off"`
	Total         float64 `json:"total"`
	AmountInWords string  `json:"amount_in_words"`
}

// Payment holds payment information.
type Payment struct {
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	IFSCCode      string `json:"ifsc_code"`
	PaymentTerms  string `json:"payment_terms"`
}

// Deprecated: ConfidenceScores are no longer populated by the parser.
// Retained for backward compatibility with existing stored documents.
type ConfidenceScores struct {
	Invoice   InvoiceConfidence   `json:"invoice"`
	Seller    PartyConfidence     `json:"seller"`
	Buyer     PartyConfidence     `json:"buyer"`
	LineItems []LineItemConfidence `json:"line_items"`
	Totals    TotalsConfidence    `json:"totals"`
	Payment   PaymentConfidence   `json:"payment"`
}

// InvoiceConfidence holds confidence for invoice header fields.
type InvoiceConfidence struct {
	InvoiceNumber float64 `json:"invoice_number"`
	InvoiceDate   float64 `json:"invoice_date"`
	DueDate       float64 `json:"due_date"`
	InvoiceType   float64 `json:"invoice_type"`
	Currency      float64 `json:"currency"`
	PlaceOfSupply         float64 `json:"place_of_supply"`
	ReverseCharge         float64 `json:"reverse_charge"`
	IRN                   float64 `json:"irn"`
	AcknowledgementNumber float64 `json:"acknowledgement_number"`
	AcknowledgementDate   float64 `json:"acknowledgement_date"`
	QRCodeData            float64 `json:"qr_code_data"`
}

// PartyConfidence holds confidence for party fields.
type PartyConfidence struct {
	Name      float64 `json:"name"`
	Address   float64 `json:"address"`
	GSTIN     float64 `json:"gstin"`
	PAN       float64 `json:"pan"`
	State     float64 `json:"state"`
	StateCode float64 `json:"state_code"`
}

// LineItemConfidence holds confidence for line item fields.
type LineItemConfidence struct {
	Description   float64 `json:"description"`
	HSNSACCode    float64 `json:"hsn_sac_code"`
	Quantity      float64 `json:"quantity"`
	Unit          float64 `json:"unit"`
	UnitPrice     float64 `json:"unit_price"`
	Discount      float64 `json:"discount"`
	TaxableAmount float64 `json:"taxable_amount"`
	CGSTRate      float64 `json:"cgst_rate"`
	CGSTAmount    float64 `json:"cgst_amount"`
	SGSTRate      float64 `json:"sgst_rate"`
	SGSTAmount    float64 `json:"sgst_amount"`
	IGSTRate      float64 `json:"igst_rate"`
	IGSTAmount    float64 `json:"igst_amount"`
	Total         float64 `json:"total"`
}

// TotalsConfidence holds confidence for totals fields.
type TotalsConfidence struct {
	Subtotal      float64 `json:"subtotal"`
	TotalDiscount float64 `json:"total_discount"`
	TaxableAmount float64 `json:"taxable_amount"`
	CGST          float64 `json:"cgst"`
	SGST          float64 `json:"sgst"`
	IGST          float64 `json:"igst"`
	Cess          float64 `json:"cess"`
	RoundOff      float64 `json:"round_off"`
	Total         float64 `json:"total"`
}

// PaymentConfidence holds confidence for payment fields.
type PaymentConfidence struct {
	BankName      float64 `json:"bank_name"`
	AccountNumber float64 `json:"account_number"`
	IFSCCode      float64 `json:"ifsc_code"`
	PaymentTerms  float64 `json:"payment_terms"`
}

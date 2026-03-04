package parser

// BuildGSTInvoicePrompt returns the extraction prompt for GST invoice documents.
func BuildGSTInvoicePrompt(documentType string) string {
	return `You are a document data extraction assistant. Analyze the provided ` + documentType + ` document and extract ALL data into the following JSON structure.

IMPORTANT INSTRUCTIONS:
- The document may span multiple pages. Extract ALL line items from every page and every section (e.g., Genuine Parts, Other Parts, Labor, Other Labor, Services, Other Charges) into a single flat "line_items" array.
- It is critical that you extract EVERY line item. Do not skip, summarize, or omit any items.
- Normalize all dates to DD-MM-YYYY format. Strip timestamps, annotations like "(On or Before)", and other non-date text.
- State codes must be exactly 2 digits, zero-padded (e.g., "07" not "7").
- If the document contains an IRN (Invoice Reference Number, a 64-character hexadecimal string), Acknowledgement Number, or Acknowledgement Date (commonly found near a QR code on e-invoices), extract them.

Return ONLY valid JSON with no markdown formatting, no code fences, no explanation — just the raw JSON object.

Return ONLY the JSON data object following this schema:
{
  "invoice": {
    "invoice_number": "",
    "invoice_date": "",
    "due_date": "",
    "invoice_type": "",
    "currency": "",
    "place_of_supply": "",
    "reverse_charge": false,
    "irn": "",
    "acknowledgement_number": "",
    "acknowledgement_date": ""
  },
  "seller": {
    "name": "", "address": "",
    "gstin": "", "pan": "",
    "state": "", "state_code": ""
  },
  "buyer": {
    "name": "", "address": "",
    "gstin": "", "pan": "",
    "state": "", "state_code": ""
  },
  "line_items": [
    {
      "description": "",
      "hsn_sac_code": "",
      "quantity": 0, "unit": "",
      "unit_price": 0, "discount": 0,
      "taxable_amount": 0,
      "cgst_rate": 0, "cgst_amount": 0,
      "sgst_rate": 0, "sgst_amount": 0,
      "igst_rate": 0, "igst_amount": 0,
      "total": 0
    }
  ],
  "totals": {
    "subtotal": 0, "total_discount": 0,
    "taxable_amount": 0,
    "cgst": 0, "sgst": 0, "igst": 0, "cess": 0,
    "round_off": 0, "total": 0,
    "amount_in_words": ""
  },
  "payment": {
    "bank_name": "",
    "account_number": "",
    "ifsc_code": "",
    "payment_terms": ""
  },
  "notes": ""
}

If a field is not present in the document, use empty string for text, 0 for numbers, and false for booleans.`
}

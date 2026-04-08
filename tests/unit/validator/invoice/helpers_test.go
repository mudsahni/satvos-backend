package invoice_test

import (
	"satvos/internal/validator/invoice"
)

// validInvoice returns a fully-valid *GSTInvoice that passes all 60 validators.
// Intrastate (seller/buyer state "29") → CGST+SGST, zero IGST.
// 1 line item: qty=10, price=100, taxable=1000, CGST=9%/90, SGST=9%/90, total=1180.
// IRN = SHA-256("29ABCDE1234F1Z5" + "INV-001" + "2024-25")
func validInvoice() *invoice.GSTInvoice {
	return &invoice.GSTInvoice{
		Invoice: invoice.InvoiceHeader{
			InvoiceNumber:         "INV-001",
			InvoiceDate:           "15/01/2025",
			DueDate:               "15/02/2025",
			InvoiceType:           "tax_invoice",
			Currency:              "INR",
			PlaceOfSupply:         "Karnataka",
			IRN:                   "fc3c01b73e8c2a0c57bfc195a264c1cdd6da3acf4fbdb5e5e06b90494d1b8568",
			AcknowledgementNumber: "1234567890",
			AcknowledgementDate:   "15/01/2025",
		},
		Seller: invoice.Party{
			Name:      "Seller Corp",
			Address:   "123 Main St, Bangalore",
			GSTIN:     "29ABCDE1234F1Z5",
			PAN:       "ABCDE1234F",
			State:     "Karnataka",
			StateCode: "29",
		},
		Buyer: invoice.Party{
			Name:      "Buyer Corp",
			Address:   "456 Oak Ave, Bangalore",
			GSTIN:     "29FGHIJ5678K1Z2",
			PAN:       "FGHIJ5678K",
			State:     "Karnataka",
			StateCode: "29",
		},
		LineItems: []invoice.LineItem{
			{
				Description:   "Widget",
				HSNSACCode:    "8471",
				Quantity:      10,
				UnitPrice:     100,
				Discount:      0,
				TaxableAmount: 1000,
				CGSTRate:      9,
				CGSTAmount:    90,
				SGSTRate:      9,
				SGSTAmount:    90,
				IGSTRate:      0,
				IGSTAmount:    0,
				Total:         1180,
			},
		},
		Totals: invoice.Totals{
			Subtotal:      1000,
			TotalDiscount: 0,
			TaxableAmount: 1000,
			CGST:          90,
			SGST:          90,
			IGST:          0,
			Cess:          0,
			RoundOff:      0,
			Total:         1180,
			AmountInWords: "One Thousand One Hundred Eighty Rupees Only",
		},
		Payment: invoice.Payment{
			BankName:      "HDFC Bank",
			AccountNumber: "1234567890",
			IFSCCode:      "HDFC0001234",
			PaymentTerms:  "Net 30",
		},
	}
}

// validInterstateInvoice returns a valid interstate invoice (different states) → IGST only.
func validInterstateInvoice() *invoice.GSTInvoice {
	inv := validInvoice()
	inv.Buyer.State = "Maharashtra"
	inv.Buyer.StateCode = "27"
	inv.Buyer.GSTIN = "27FGHIJ5678K1Z2"
	// Switch to IGST
	inv.LineItems[0].CGSTRate = 0
	inv.LineItems[0].CGSTAmount = 0
	inv.LineItems[0].SGSTRate = 0
	inv.LineItems[0].SGSTAmount = 0
	inv.LineItems[0].IGSTRate = 18
	inv.LineItems[0].IGSTAmount = 180
	inv.LineItems[0].Total = 1180
	inv.Totals.CGST = 0
	inv.Totals.SGST = 0
	inv.Totals.IGST = 180
	inv.Totals.Total = 1180
	return inv
}

func findRequiredValidator(key string) *invoice.BuiltinValidator {
	for _, v := range invoice.AllBuiltinValidators() {
		if v.RuleKey() == key {
			return v
		}
	}
	return nil
}

func findFormatValidator(key string) *invoice.BuiltinValidator {
	return findRequiredValidator(key) // same lookup; AllBuiltinValidators has all
}

func findMathValidator(key string) *invoice.BuiltinValidator {
	return findRequiredValidator(key)
}

func findCrossFieldValidator(key string) *invoice.BuiltinValidator {
	return findRequiredValidator(key)
}

func findLogicalValidator(key string) *invoice.BuiltinValidator {
	return findRequiredValidator(key)
}

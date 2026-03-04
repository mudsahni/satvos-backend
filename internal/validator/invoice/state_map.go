package invoice

import "strings"

// IndianStateCodeToName maps GST state codes to official state/UT names.
var IndianStateCodeToName = map[string]string{
	"01": "Jammu and Kashmir",
	"02": "Himachal Pradesh",
	"03": "Punjab",
	"04": "Chandigarh",
	"05": "Uttarakhand",
	"06": "Haryana",
	"07": "Delhi",
	"08": "Rajasthan",
	"09": "Uttar Pradesh",
	"10": "Bihar",
	"11": "Sikkim",
	"12": "Arunachal Pradesh",
	"13": "Nagaland",
	"14": "Manipur",
	"15": "Mizoram",
	"16": "Tripura",
	"17": "Meghalaya",
	"18": "Assam",
	"19": "West Bengal",
	"20": "Jharkhand",
	"21": "Odisha",
	"22": "Chhattisgarh",
	"23": "Madhya Pradesh",
	"24": "Gujarat",
	"25": "Daman and Diu",
	"26": "Dadra and Nagar Haveli",
	"27": "Maharashtra",
	"28": "Andhra Pradesh",
	"29": "Karnataka",
	"30": "Goa",
	"31": "Lakshadweep",
	"32": "Kerala",
	"33": "Tamil Nadu",
	"34": "Puducherry",
	"35": "Andaman and Nicobar Islands",
	"36": "Telangana",
	"37": "Andhra Pradesh (New)",
	"38": "Ladakh",
}

// stateNameToCode maps normalized state names and common abbreviations to GST codes.
var stateNameToCode map[string]string

func init() {
	stateNameToCode = make(map[string]string, len(IndianStateCodeToName)*2)
	for code, name := range IndianStateCodeToName {
		stateNameToCode[strings.ToLower(name)] = code
	}
	// Common abbreviations
	stateNameToCode["jk"] = "01"
	stateNameToCode["j&k"] = "01"
	stateNameToCode["hp"] = "02"
	stateNameToCode["pb"] = "03"
	stateNameToCode["ch"] = "04"
	stateNameToCode["uk"] = "05"
	stateNameToCode["hr"] = "06"
	stateNameToCode["dl"] = "07"
	stateNameToCode["rj"] = "08"
	stateNameToCode["up"] = "09"
	stateNameToCode["br"] = "10"
	stateNameToCode["sk"] = "11"
	stateNameToCode["ar"] = "12"
	stateNameToCode["nl"] = "13"
	stateNameToCode["mn"] = "14"
	stateNameToCode["mz"] = "15"
	stateNameToCode["tr"] = "16"
	stateNameToCode["ml"] = "17"
	stateNameToCode["as"] = "18"
	stateNameToCode["wb"] = "19"
	stateNameToCode["jh"] = "20"
	stateNameToCode["or"] = "21"
	stateNameToCode["od"] = "21"
	stateNameToCode["cg"] = "22"
	stateNameToCode["mp"] = "23"
	stateNameToCode["gj"] = "24"
	stateNameToCode["dd"] = "25"
	stateNameToCode["dn"] = "26"
	stateNameToCode["mh"] = "27"
	stateNameToCode["ap"] = "37"
	stateNameToCode["ka"] = "29"
	stateNameToCode["ga"] = "30"
	stateNameToCode["ld"] = "31"
	stateNameToCode["kl"] = "32"
	stateNameToCode["tn"] = "33"
	stateNameToCode["py"] = "34"
	stateNameToCode["an"] = "35"
	stateNameToCode["ts"] = "36"
	stateNameToCode["la"] = "38"
}

// LookupStateCode attempts to resolve a state name/abbreviation to its GST code.
// Returns the code and true if found, empty string and false otherwise.
func LookupStateCode(stateName string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(stateName))
	code, ok := stateNameToCode[normalized]
	return code, ok
}

// stateNameCodeCheck validates that a party's state name matches their state code.
func stateNameCodeCheck(party, stateName, stateCode string) []ValidationResult {
	fieldPath := party + ".state"
	if stateName == "" || stateCode == "" {
		return []ValidationResult{{
			Passed:    true,
			FieldPath: fieldPath,
			Message:   "Cross-field: " + party + " State Name-Code Match: fields missing, skipping",
		}}
	}

	expectedCode, found := LookupStateCode(stateName)
	if !found {
		return []ValidationResult{{
			Passed:    true,
			FieldPath: fieldPath,
			Message:   "Cross-field: " + party + " State Name-Code Match: state name not recognized, skipping",
		}}
	}

	passed := expectedCode == stateCode
	msg := "Cross-field: " + party + " State Name-Code Match: state name matches state code"
	if !passed {
		msg = "Cross-field: " + party + " State Name-Code Match: state name '" + stateName + "' maps to code " + expectedCode + " but state_code is " + stateCode
	}
	return []ValidationResult{{
		Passed:        passed,
		FieldPath:     fieldPath,
		ExpectedValue: expectedCode,
		ActualValue:   stateCode,
		Message:       msg,
	}}
}

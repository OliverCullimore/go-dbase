package dbase

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

// Converts raw column data to the correct type for the given column
// For C and M columns a charset conversion is done
// For M columns the data is read from the memo file
// At this moment not all FoxPro column types are supported.
// When reading column values, the value returned by this package is always `interface{}`.
//
// The supported column types with their return Go types are:
//
//	Column Type >> Column Type Name >> Golang type
//
//	B  >>  Double  >>  float64
//	C  >>  Character  >>  string
//	D  >>  Date  >>  time.Time
//	F  >>  Float  >>  float64
//	I  >>  Integer  >>  int32
//	L  >>  Logical  >>  bool
//	M  >>  Memo   >>  string
//	M  >>  Memo (Binary)  >>  []byte
//	N  >>  Numeric (0 decimals)  >>  int64
//	N  >>  Numeric (with decimals)  >>  float64
//	T  >>  DateTime  >>  time.Time
//	Y  >>  Currency  >>  float64
//
// This package contains the functions to convert a dbase database entry as byte array into a row struct
// with the columns converted into the corresponding data types.
func (dbf *DBF) dataToValue(raw []byte, column *Column) (interface{}, error) {
	// Not all column types have been implemented because we don't use them in our DBFs
	// Extend this function if needed
	if len(raw) != int(column.Length) {
		return nil, fmt.Errorf("dbase-interpreter-datatovalue-1:FAILED:invalid length %v Bytes != %v Bytes at column field: %v", len(raw), column.Length, column.Name())
	}
	switch column.Type() {
	case "M":
		// M values contain the address in the FPT file from where to read data
		memo, isText, err := dbf.parseMemo(raw)
		if err != nil {
			return nil, fmt.Errorf("dbase-interpreter-datatovalue-2:FAILED:parsing memo failed at column field: %v failed with error: %w", column.Name(), err)
		}
		if isText {
			return string(memo), nil
		}
		return memo, nil
	case "C":
		// C values are stored as strings, the returned string is not trimmed
		str, err := toUTF8String(raw, dbf.converter)
		if err != nil {
			return str, fmt.Errorf("dbase-interpreter-datatovalue-4:FAILED:parsing to utf8 string failed at column field: %v failed with error: %w", column.Name(), err)
		}
		return str, nil
	case "I":
		// I values are stored as numeric values
		return int32(binary.LittleEndian.Uint32(raw)), nil
	case "B":
		// B (double) values are stored as numeric values
		return math.Float64frombits(binary.LittleEndian.Uint64(raw)), nil
	case "D":
		// D values are stored as string in format YYYYMMDD, convert to time.Time
		date, err := parseDate(raw)
		if err != nil {
			return date, fmt.Errorf("dbase-interpreter-datatovalue-5:FAILED:parsing to date at column field: %v failed with error: %w", column.Name(), err)
		}
		return date, nil
	case "T":
		// T values are stores as two 4 byte integers
		//  integer one is the date in julian format
		//  integer two is the number of milliseconds since midnight
		// Above info from http://fox.wikis.com/wc.dll?Wiki~DateTime
		dateTime, err := parseDateTime(raw)
		if err != nil {
			return dateTime, fmt.Errorf("dbase-interpreter-datatovalue-6:FAILED:parsing date time at column field: %v failed with error: %w", column.Name(), err)
		}
		return dateTime, nil
	case "L":
		// L values are stored as strings T or F, we only check for T, the rest is false...
		return string(raw) == "T", nil
	case "V":
		// V values just return the raw value
		return raw, nil
	case "Y":
		// Y values are currency values stored as ints with 4 decimal places
		return float64(binary.LittleEndian.Uint64(raw)) / 10000, nil
	case "N":
		// N values are stored as string values, if no decimals return as int64, if decimals treat as float64
		if column.Decimals == 0 {
			i, err := parseNumericInt(raw)
			if err != nil {
				return i, fmt.Errorf("dbase-interpreter-datatovalue-7:FAILED:parsing numeric int at column field: %v failed with error: %w", column.Name(), err)
			}
			return i, nil
		}
		fallthrough // same as "F"
	case "F":
		// F values are stored as string values
		f, err := parseFloat(raw)
		if err != nil {
			return f, fmt.Errorf("dbase-interpreter-datatovalue-8:FAILED:parsing float at column field: %v failed with error: %w", column.Name(), err)
		}
		return f, nil
	default:
		return nil, fmt.Errorf("dbase-interpreter-datatovalue-9:FAILED:Unsupported column data type: %s", column.Type())
	}
}

// Converts column data to the byte representation
// For M values the data has to be written to the memo file
func (dbf *DBF) valueToByteRepresentation(field *Field) ([]byte, error) {
	switch field.Type() {
	case "M":
		address, err := dbf.getMRepresentation(field)
		if err != nil {
			return nil, fmt.Errorf("dbase-interpreter-valuetobyterepresentation-1:FAILED:%w", err)
		}
		return address, nil
	case "C":
		// C values are stored as strings, the returned string is not trimmed
		raw, err := dbf.getCRepresentation(field)
		if err != nil {
			return nil, fmt.Errorf("dbase-interpreter-valuetobyterepresentation-2:FAILED:%w", err)
		}
		return raw, nil
	case "I":
		// I values (int32)
		raw, err := dbf.getIRepresentation(field)
		if err != nil {
			return nil, fmt.Errorf("dbase-interpreter-valuetobyterepresentation-3:FAILED:%w", err)
		}
		return raw, nil
	case "Y":
		// Y (currency)
		raw, err := dbf.getYRepresentation(field)
		if err != nil {
			return nil, fmt.Errorf("dbase-interpreter-valuetobyterepresentation-4:FAILED:%w", err)
		}
		return raw, nil
	case "F":
		// F (Float)
		raw, err := dbf.getFRepresentation(field)
		if err != nil {
			return nil, fmt.Errorf("dbase-interpreter-valuetobyterepresentation-5:FAILED:%w", err)
		}
		return raw, nil
	case "B":
		// B (double)
		raw, err := dbf.getBRepresentation(field)
		if err != nil {
			return nil, fmt.Errorf("dbase-interpreter-valuetobyterepresentation-6:FAILED:%w", err)
		}
		return raw, nil
	case "D":
		// D values are stored as string in format YYYYMMDD, convert to time.Time
		raw, err := dbf.getDRepresentation(field)
		if err != nil {
			return nil, fmt.Errorf("dbase-interpreter-valuetobyterepresentation-7:FAILED:%w", err)
		}
		return raw, nil
	case "T":
		// T values are stores as two 4 byte integers
		//  integer one is the date in julian format
		//  integer two is the number of milliseconds since midnight
		// Above info from http://fox.wikis.com/wc.dll?Wiki~DateTime
		raw, err := dbf.getTRepresentation(field)
		if err != nil {
			return nil, fmt.Errorf("dbase-interpreter-valuetobyterepresentation-8:FAILED:%w", err)
		}
		return raw, nil
	case "L":
		// L (bool) values are stored as strings T or F, we only check for T, the rest is false...
		raw, err := dbf.getLRepresentation(field)
		if err != nil {
			return nil, fmt.Errorf("dbase-interpreter-valuetobyterepresentation-9:FAILED:%w", err)
		}
		return raw, nil
	case "V":
		// V values just return the raw value
		raw, ok := field.value.([]byte)
		if !ok {
			return nil, fmt.Errorf("dbase-interpreter-valuetobyterepresentation-10:FAILED:invalid data type %T, expected []byte at column field: %v", field.value, field.Name())
		}
		return raw, nil
	case "N":
		// N values are stored as string values, if no decimals return as int64, if decimals treat as float64
		raw, err := dbf.getNRepresentation(field)
		if err != nil {
			return nil, fmt.Errorf("dbase-interpreter-valuetobyterepresentation-11:FAILED:%w", err)
		}
		return raw, nil
	default:
		return nil, fmt.Errorf("dbase-interpreter-valuetobyterepresentation-12:FAILED:Unsupported column data type: %s at column field: %v", field.column.Type(), field.Name())
	}
}

func (dbf *DBF) getMRepresentation(field *Field) ([]byte, error) {
	memo := make([]byte, 0)
	txt := false
	s, sok := field.value.(string)
	if sok {
		memo = []byte(s)
		txt = true
	}
	m, ok := field.value.([]byte)
	if ok {
		memo = m
		txt = false
	}
	if !ok && !sok {
		return nil, fmt.Errorf("dbase-interpreter-getmrepresentation-1:FAILED:invalid type for memo field: %T", field.value)
	}
	// Write the memo to the memo file
	address, err := dbf.writeMemo(memo, txt, len(memo))
	if err != nil {
		return nil, fmt.Errorf("dbase-interpreter-getmrepresentation-2:FAILED:writing to memo file at column field: %v failed with error: %w", field.Name(), err)
	}
	return address, nil
}

func (dbf *DBF) getCRepresentation(field *Field) ([]byte, error) {
	// C values are stored as strings, the returned string is not trimmed
	c, ok := field.value.(string)
	if !ok {
		return nil, fmt.Errorf("dbase-interpreter-getcrepresentation-1:FAILED:invalid data type %T, expected string on column field: %v", field.value, field.Name())
	}
	raw := make([]byte, field.column.Length)
	bin, err := fromUtf8String([]byte(c), dbf.converter)
	if err != nil {
		return nil, fmt.Errorf("dbase-interpreter-getcrepresentation-2:FAILED:parsing from utf8 string at column field: %v failed with error %w", field.Name(), err)
	}
	bin = appendSpaces(bin, int(field.column.Length))
	copy(raw, bin)
	if len(raw) > int(field.column.Length) {
		return nil, fmt.Errorf("dbase-interpreter-getcrepresentation-3:FAILED:invalid length %v Bytes > %v Bytes at column field: %v", len(raw), field.column.Length, field.Name())
	}
	return raw, nil
}

func (dbf *DBF) getIRepresentation(field *Field) ([]byte, error) {
	// I values (int32)
	i, ok := field.value.(int32)
	if !ok {
		f, ok := field.value.(float64)
		if !ok {
			return nil, fmt.Errorf("dbase-interpreter-getirepresentation-1:FAILED:invalid data type %T, expected int32 at column field: %v", field.value, field.Name())
		}
		// check for lower and uppper bounds
		if f > 0 && f <= math.MaxInt32 {
			i = int32(f)
		}
	}
	raw := make([]byte, field.column.Length)
	bin, err := toBinary(i)
	if err != nil {
		return nil, fmt.Errorf("dbase-interpreter-getirepresentation-2:FAILED:converting to binary at column field: %v failed with error: %w", field.Name(), err)
	}
	copy(raw, bin)
	if len(raw) != int(field.column.Length) {
		return nil, fmt.Errorf("dbase-interpreter-getirepresentation-3:FAILED:invalid length %v Bytes != %v Bytes at column field: %v", len(raw), field.column.Length, field.Name())
	}
	return raw, nil
}

func (dbf *DBF) getYRepresentation(field *Field) ([]byte, error) {
	f, ok := field.value.(float64)
	if !ok {
		return nil, fmt.Errorf("dbase-interpreter-getyrepresentation-1:FAILED:invalid data type %T, expected float64 at column field: %v", field.value, field.Name())
	}
	// Cast to int64 and multiply by 10000
	i := int64(f * 10000)
	raw := make([]byte, field.column.Length)
	bin, err := toBinary(i)
	if err != nil {
		return nil, fmt.Errorf("dbase-interpreter-getyrepresentation-2:FAILED:converting to binary at column field: %v failed with error: %w", field.Name(), err)
	}
	copy(raw, bin)
	if len(raw) != int(field.column.Length) {
		return nil, fmt.Errorf("dbase-interpreter-getyrepresentation-3:FAILED:invalid length %v Bytes != %v Bytes at column field: %v", len(raw), field.column.Length, field.Name())
	}
	return raw, nil
}

func (dbf *DBF) getFRepresentation(field *Field) ([]byte, error) {
	b, ok := field.value.(float64)
	if !ok {
		return nil, fmt.Errorf("dbase-interpreter-getfrepresentation-1:FAILED:invalid data type %T, expected float64 at column field: %v", field.value, field.Name())
	}
	var bin []byte
	if b == float64(int64(b)) {
		// if the value is an integer, store as integer
		bin = []byte(fmt.Sprintf("%d", int64(b)))
	} else {
		// if the value is a float, store as float
		expression := fmt.Sprintf("%%.%df", field.column.Decimals)
		bin = []byte(fmt.Sprintf(expression, field.value))
	}
	return prependSpaces(bin, int(field.column.Length)), nil
}

func (dbf *DBF) getBRepresentation(field *Field) ([]byte, error) {
	b, ok := field.value.(float64)
	if !ok {
		return nil, fmt.Errorf("dbase-interpreter-getbrepresentation-1:FAILED:invalid data type %T, expected float64 at column field: %v", field.value, field.Name())
	}
	raw := make([]byte, field.column.Length)
	bin, err := toBinary(b)
	if err != nil {
		return nil, fmt.Errorf("dbase-interpreter-getbrepresentation-2:FAILED:converting to binary at column field: %v failed with error: %w", field.Name(), err)
	}
	copy(raw, bin)
	if len(raw) != int(field.column.Length) {
		return nil, fmt.Errorf("dbase-interpreter-getbrepresentation-3:FAILED:invalid length %v Bytes != %v Bytes at column field: %v", len(raw), field.column.Length, field.Name())
	}
	return raw, nil
}

func (dbf *DBF) getDRepresentation(field *Field) ([]byte, error) {
	d, ok := field.value.(time.Time)
	if !ok {
		s, ok := field.value.(string)
		if !ok {
			return nil, fmt.Errorf("dbase-interpreter-getdrepresentation-1:FAILED:invalid data type %T, expected time.Time at column field: %v", field.value, field.Name())
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("dbase-interpreter-getdrepresentation-2:FAILED:parsing time failed at column field: %v failed with error: %w", field.Name(), err)
		}
		d = t
	}
	raw := make([]byte, field.column.Length)
	bin := []byte(d.Format("20060102"))
	copy(raw, bin)
	if len(raw) != int(field.column.Length) {
		return nil, fmt.Errorf("dbase-interpreter-getdrepresentation-3:FAILED:invalid length %v Bytes != %v Bytes at column field: %v", len(raw), field.column.Length, field.Name())
	}
	return raw, nil
}

func (dbf *DBF) getTRepresentation(field *Field) ([]byte, error) {
	t, ok := field.value.(time.Time)
	if !ok {
		s, ok := field.value.(string)
		if !ok {
			return nil, fmt.Errorf("dbase-interpreter-gettrepresentation-1:FAILED:invalid data type %T, expected time.Time at column field: %v", field.value, field.Name())
		}
		parsedTime, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("dbase-interpreter-gettrepresentation-2:FAILED:parsing time failed at column field: %v failed with error: %w", field.Name(), err)
		}
		t = parsedTime
	}
	raw := make([]byte, 8)
	i := YMD2JD(t.Year(), int(t.Month()), t.Day())
	date, err := toBinary(uint64(i))
	if err != nil {
		return nil, fmt.Errorf("dbase-interpreter-gettrepresentation-3:FAILED:Time conversion at column field: %v failed with error: %w", field.Name(), err)
	}
	copy(raw[:4], date)
	millis := t.Hour()*3600000 + t.Minute()*60000 + t.Second()*1000 + t.Nanosecond()/1000000
	time, err := toBinary(uint64(millis))
	if err != nil {
		return nil, fmt.Errorf("dbase-interpreter-gettrepresentation-4:FAILED:Binary conversion at column field: %v failed with error: %w", field.Name(), err)
	}
	copy(raw[4:], time)
	if len(raw) != int(field.column.Length) {
		return nil, fmt.Errorf("dbase-interpreter-gettrepresentation-5:FAILED:invalid length %v Bytes != %v Bytes at column field: %v", len(raw), field.column.Length, field.Name())
	}
	return raw, nil
}

func (dbf *DBF) getLRepresentation(field *Field) ([]byte, error) {
	l, ok := field.value.(bool)
	if !ok {
		return nil, fmt.Errorf("dbase-interpreter-getlrepresentation-1:FAILED:invalid data type %T, expected bool at column field: %v", field.value, field.Name())
	}
	raw := []byte("F")
	if l {
		return []byte("T"), nil
	}
	return raw, nil
}

func (dbf *DBF) getNRepresentation(field *Field) ([]byte, error) {
	// N values are stored as string values, if no decimals return as int64, if decimals treat as float64
	bin := make([]byte, 0)
	f, fok := field.value.(float64)
	if fok {
		if f == float64(int64(f)) {
			// if the value is an integer, store as integer
			bin = []byte(fmt.Sprintf("%d", int64(f)))
		} else {
			// if the value is a float, store as float
			expression := fmt.Sprintf("%%.%df", field.column.Decimals)
			bin = []byte(fmt.Sprintf(expression, field.value))
		}
	}
	_, iok := field.value.(int64)
	if iok {
		bin = []byte(fmt.Sprintf("%d", field.value))
	}
	if !iok && !fok {
		return nil, fmt.Errorf("dbase-interpreter-getnrepresentation-1:FAILED:invalid data type %T, expected int64 or float64 at column field: %v", field.value, field.Name())
	}
	return prependSpaces(bin, int(field.column.Length)), nil
}

func toBinary(data interface{}) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, data)
	if err != nil {
		return nil, fmt.Errorf("dbase-interpreter-tobinary-1:FAILED:%w", err)
	}
	return buf.Bytes(), nil
}

package types

func NewSignBytes(data []byte) *SignBytes {
	return &SignBytes{
		data: data,
	}
}

type SignBytes struct {
	data []byte `rlp:"-"`
	Sign SignContent
}

func (sb *SignBytes) Bytes() []byte {
	return sb.data
}

// Impl sign interface
func (sb *SignBytes) GetSignStruct() []interface{} {
	return []interface{}{sb}
}

func (sb *SignBytes) GetRSVBySignatureHash() []byte {
	return sb.Sign.Get()
}

func (sb *SignBytes) SetSign(sign []byte) {
	sb.Sign.Set(sign)
}

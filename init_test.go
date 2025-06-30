package golden

func init() {
	RegisterBaseBlock(func() BlockType {
		return new(BaseData)
	})
	RegisterBaseBlock(func() BlockType { return new(BaseResource) })
	RegisterBlock(new(DummyData))
	RegisterBlock(new(DummyResource))
	RegisterBlock(new(PureApplyBlock))
	RegisterBlock(new(PureApplyBlock2))
	RegisterBlock(new(DummyRootBlock))
	RegisterBlock(new(SelfRefRootBlock))
	RegisterCustomGoTypeMapping()
}

func RegisterCustomGoTypeMapping() {
	AddCustomTypeMapping[*SelfRefBlock](new(SelfRefBlock).customCtyType(100))
}

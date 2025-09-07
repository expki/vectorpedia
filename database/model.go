package database

type Page struct {
	ID        uint64  `gorm:"primaryKey;not null"`
	TitleID   uint64  `gorm:"not null"`
	Title     Title   `gorm:"foreignKey:TitleID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	ContentID uint64  `gorm:"not null"`
	Content   Content `gorm:"foreignKey:ContentID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	SummaryID uint64  `gorm:"not null"`
	Summary   Summary `gorm:"foreignKey:SummaryID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

type Title struct {
	ID          uint64    `gorm:"primaryKey;not null"`
	Value       string    `gorm:"not null"`
	EmbeddingID uint64    `gorm:"not null"`
	Embedding   Embedding `gorm:"foreignKey:EmbeddingID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

type Content struct {
	ID         uint64      `gorm:"primaryKey;not null"`
	Value      string      `gorm:"not null"`
	Embeddings []Embedding `gorm:"many2many:content_embeddings;"`
}

type Summary struct {
	ID          uint64    `gorm:"primaryKey;not null"`
	Value       string    `gorm:"not null"`
	EmbeddingID uint64    `gorm:"not null"`
	Embedding   Embedding `gorm:"foreignKey:EmbeddingID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

type Embedding struct {
	ID     uint64 `gorm:"primaryKey;not null"`
	Vector []byte `gorm:"not null"`
}

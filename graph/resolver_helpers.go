package graph

import (
	"strconv"

	"github.com/Cityboypenguin/SPACE-server/graph/model"
	domainmodel "github.com/Cityboypenguin/SPACE-server/model"
)

func toGraphQLPost(p *domainmodel.Post, author *domainmodel.User) *model.Post {
	return &model.Post{
		ID:        strconv.FormatInt(p.ID, 10),
		Author:    toGraphQLUser(author),
		Content:   p.Content,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

func toGraphQLUser(u *domainmodel.User) *model.User {
	return &model.User{
		ID:        strconv.FormatInt(u.ID, 10),
		Name:      u.Name,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

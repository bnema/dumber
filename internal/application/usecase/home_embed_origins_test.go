package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/domain/entity"
)

func TestHomeUseCase_LoadHome_PublishesEmbedOrigins(t *testing.T) {
	ctx := context.Background()
	layout := entity.DefaultHomeLayout()
	layout.Groups = []entity.HomeGroup{{Name: "G", Services: []entity.HomeService{
		{ID: "graf", Name: "Grafana", Widget: &entity.HomeServiceWidget{
			Type: entity.HomeServiceWidgetEmbed, URL: "http://10.0.0.2:3000/d/x",
		}},
		{ID: "api", Name: "API", Widget: &entity.HomeServiceWidget{
			Type: entity.HomeServiceWidgetAPI, URL: "http://10.0.0.9/api",
		}},
	}}}

	source := mocks.NewMockHomeConfigSource(t)
	source.EXPECT().Load(ctx).Return(port.HomeConfigSnapshot{Layout: layout}, nil)
	origins := mocks.NewMockHomeEmbedOriginSetter(t)
	origins.EXPECT().SetHomeEmbedOrigins([]string{"http://10.0.0.2:3000"}).Once()

	_, err := NewHomeUseCase(HomeDeps{Source: source, EmbedOrigins: origins}).LoadHome(ctx)
	require.NoError(t, err)
}

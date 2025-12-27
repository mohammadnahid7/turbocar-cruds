package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"
	"wegugin/auth"
	pb "wegugin/genproto/cruds"
	"wegugin/storage/postgres/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type imageData struct {
	ID         string `json:"id"`
	CarID      string `json:"car_id"`
	Filename   string `json:"filename"`
	UploadedAt string `json:"uploaded_at"`
}

// ---------------------- HELPER FUNCTIONS ----------------------
func (s *CarService) getUserIDFromContext(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		s.logger.Error("metadata not found in context")
		return "", status.Error(codes.Unauthenticated, "authentication required")
	}

	authHeader := md.Get("Authorization")
	if len(authHeader) == 0 {
		authHeader = md.Get("authorization")
	}
	if len(authHeader) == 0 {
		s.logger.Error("Authorization header missing")
		return "", status.Error(codes.Unauthenticated, "Authorization token required")
	}

	token := strings.TrimPrefix(authHeader[0], "Bearer ")
	userID, _, err := auth.GetUserIdFromToken(token)
	if err != nil {
		s.logger.Error("invalid token", "error", err)
		return "", status.Error(codes.Unauthenticated, "invalid token")
	}

	return userID, nil
}

func (s *CarService) checkCarOwnership(ctx context.Context, carID string) error {
	userID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return err
	}

	res, err := s.CheckCarOwnership(ctx, &pb.BoolCheckCar{
		UserId: userID, // <- userID ni qo'shamiz
		CarId:  carID,
	})

	if err != nil || !res.Result {
		return status.Error(codes.PermissionDenied, "permission denied")
	}
	return nil
}

func convertFloatToNumeric(price float64) (pgtype.Numeric, error) {
	priceStr := fmt.Sprintf("%.2f", price)
	parts := strings.Split(priceStr, ".")
	if len(parts) != 2 {
		return pgtype.Numeric{}, fmt.Errorf("invalid price format")
	}

	combined := parts[0] + parts[1]
	value := new(big.Int)
	if _, ok := value.SetString(combined, 10); !ok {
		return pgtype.Numeric{}, fmt.Errorf("invalid price value")
	}

	return pgtype.Numeric{
		Int:   value,
		Exp:   -2,
		Valid: true,
	}, nil
}

func (s *CarService) convertDBCarToProto(dbCar sqlc.CreateCarRow) *pb.Car {
	price, _ := convertNumericToFloat(dbCar.Price)
	return &pb.Car{
		Id:           dbCar.ID,
		Type:         dbCar.Type,
		Make:         dbCar.Make,
		Model:        dbCar.Model,
		Year:         dbCar.Year,
		Color:        dbCar.Color,
		Mileage:      dbCar.Mileage,
		Price:        price,
		Description:  dbCar.Description.String,
		Available:    dbCar.Available.Bool,
		OwnerId:      dbCar.OwnerID,
		Location:     dbCar.Location,
		ReviewsCount: int32(dbCar.ReviewsCount.Int32),
		CreatedAt:    dbCar.CreatedAt.Time.Format("2006-01-02 15:04:05"),
		UpdatedAt:    dbCar.UpdatedAt.Time.Format("2006-01-02 15:04:05"),
	}
}

func convertNumericToFloat(n interface{}) (float64, error) {
	pgNum, ok := n.(pgtype.Numeric)
	if !ok || !pgNum.Valid {
		return 0, nil
	}

	num := new(big.Float).SetInt(pgNum.Int)
	exp := pgNum.Exp

	// 10^|exp| ni hisoblash
	scale := new(big.Int).Exp(
		big.NewInt(10),
		big.NewInt(int64(-exp)), // exp manfiy, masalan -2 uchun 10^2
		nil,
	)

	// Scale ni floatga o'tkazamiz
	scaleFloat := new(big.Float).SetInt(scale)

	// Bo'lish amalini bajaramiz
	result, _ := num.Quo(num, scaleFloat).Float64()

	return result, nil
}

// Umumiy rasmlarni qayta ishlovchi funksiya
func (s *CarService) processImages(jsonData []byte) []*pb.Image {
	var images []*pb.Image
	if len(jsonData) > 0 {
		var dbImages []imageData
		if err := json.Unmarshal(jsonData, &dbImages); err != nil {
			s.logger.Warn("failed to parse car images", "error", err)
		} else {
			images = make([]*pb.Image, len(dbImages))
			for i, img := range dbImages {
				images[i] = &pb.Image{
					Id:         img.ID,
					CarId:      img.CarID,
					Filename:   img.Filename,
					UploadedAt: img.UploadedAt,
				}
			}
		}
	}
	return images
}

// ListCars uchun konvertatsiya
func (s *CarService) convertListCarToProto(dbCar sqlc.ListCarsRow) *pb.Car {
	price, _ := convertNumericToFloat(dbCar.Price)

	return &pb.Car{
		Id:           dbCar.ID,
		Type:         dbCar.Type,
		Make:         dbCar.Make,
		Model:        dbCar.Model,
		Year:         dbCar.Year,
		Color:        dbCar.Color,
		Mileage:      dbCar.Mileage,
		Price:        price,
		Description:  dbCar.Description.String,
		Available:    dbCar.Available.Bool,
		OwnerId:      dbCar.OwnerID,
		Location:     dbCar.Location,
		ReviewsCount: int32(dbCar.ReviewsCount.Int32),
		Images:       s.processImages(dbCar.Images),
		CreatedAt:    dbCar.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:    dbCar.UpdatedAt.Time.Format(time.RFC3339),
	}
}

func (s *CarService) convertPriceToNumeric(price float64) (pgtype.Numeric, error) {
	if price == 0 {
		return pgtype.Numeric{Valid: false}, nil
	}

	// Floatni stringga aylantirish (2 ta kasr bilan)
	priceStr := fmt.Sprintf("%.2f", price)

	var numeric pgtype.Numeric
	err := numeric.Scan(priceStr)
	if err != nil {
		s.logger.Error("price conversion error",
			"price", priceStr,
			"error", err)
		return pgtype.Numeric{}, fmt.Errorf("invalid price value")
	}
	numeric.Valid = true
	return numeric, nil
}

// SearchCar uchun konvertatsiya
func (s *CarService) convertSearchCarToProto(dbCar sqlc.SearchCarRow) *pb.Car {
	price, _ := convertNumericToFloat(dbCar.Price)

	return &pb.Car{
		Id:           dbCar.ID,
		Type:         dbCar.Type,
		Make:         dbCar.Make,
		Model:        dbCar.Model,
		Year:         dbCar.Year,
		Color:        dbCar.Color,
		Mileage:      dbCar.Mileage,
		Price:        price,
		Description:  dbCar.Description.String,
		Available:    dbCar.Available.Bool,
		OwnerId:      dbCar.OwnerID,
		Location:     dbCar.Location,
		ReviewsCount: int32(dbCar.ReviewsCount.Int32),
		Images:       s.processImages(dbCar.Images),
		CreatedAt:    dbCar.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:    dbCar.UpdatedAt.Time.Format(time.RFC3339),
	}
}

// GetCarById uchun konvertatsiya (yangi versiya)
func (s *CarService) convertDBCarToProtoWithImages(dbCar sqlc.GetCarByIdRow) *pb.Car {
	price, _ := convertNumericToFloat(dbCar.Price)

	return &pb.Car{
		Id:           dbCar.ID,
		Type:         dbCar.Type,
		Make:         dbCar.Make,
		Model:        dbCar.Model,
		Year:         dbCar.Year,
		Color:        dbCar.Color,
		Mileage:      dbCar.Mileage,
		Price:        price,
		Description:  dbCar.Description.String,
		Available:    dbCar.Available.Bool,
		OwnerId:      dbCar.OwnerID,
		Location:     dbCar.Location,
		ReviewsCount: int32(dbCar.ReviewsCount.Int32),
		Images:       s.processImages(dbCar.Images),
		CreatedAt:    dbCar.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:    dbCar.UpdatedAt.Time.Format(time.RFC3339),
	}
}

// ---------------------- HELPER FUNCTIONS ----------------------

func (s *CarService) checkSavedCarOwnership(ctx context.Context, savedCarID string) error {
	userID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return err
	}

	res, err := s.CheckSavedCarOwnership(ctx, &pb.BoolCheckSavedCars{
		UserId:     userID,
		SavedCarId: savedCarID,
	})

	if err != nil || !res.Result {
		return status.Error(codes.PermissionDenied, "permission denied")
	}
	return nil
}

func (s *CarService) checkCommentOwnership(ctx context.Context, commentId string) error {
	userID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return err
	}

	res, err := s.CheckCommentOwnership(ctx, &pb.BoolCheckComment{
		UserId:    userID,
		CommentId: commentId,
	})

	if err != nil || !res.Result {
		return status.Error(codes.PermissionDenied, "permission denied")
	}
	return nil
}

// ---------------------- HELPER FUNCTIONS ----------------------

// SQL natijasini protobuf formatiga o'tkazish
func (s *CarService) convertDBNotificationToProto(dbNotif interface{}) *pb.Notification {
	var notif *pb.Notification

	switch v := dbNotif.(type) {
	case sqlc.GetNotificationsByUserRow:
		notif = &pb.Notification{
			Id:        v.ID,
			UserId:    v.UserID,
			Type:      v.Type,
			Message:   v.Message,
			Seen:      v.Seen.Bool,
			CreatedAt: v.CreatedAt.Time.Format(time.RFC3339),
		}
	case sqlc.GetUnreadNotificationsByUserRow:
		notif = &pb.Notification{
			Id:        v.ID,
			UserId:    v.UserID,
			Type:      v.Type,
			Message:   v.Message,
			Seen:      v.Seen.Bool,
			CreatedAt: v.CreatedAt.Time.Format(time.RFC3339),
		}
	default:
		s.logger.Error("unknown notification type", "type", fmt.Sprintf("%T", dbNotif))
		return &pb.Notification{}
	}

	return notif
}

// ---------------------- HELPER FUNCTIONS ----------------------

// SQL natijasini protobuf formatiga o'tkazish
func (s *CarService) convertDBMessageToProto(dbMsg sqlc.CreateMessageRow) *pb.Message {
	return &pb.Message{
		Id:          dbMsg.ID,
		SenderId:    dbMsg.SenderID,
		RecipientId: dbMsg.RecipientID,
		Content:     dbMsg.Content,
		Read:        dbMsg.Read.Bool,
		CreatedAt:   dbMsg.CreatedAt.Time.Format(time.RFC3339),
	}
}

// JSON ma'lumotni protobuf formatiga o'tkazish
func (s *CarService) convertRawMessage(raw map[string]interface{}) (*pb.Message, error) {
	// Vaqt formatini konvertatsiya qilish
	createdAt, err := time.Parse(time.RFC3339, raw["created_at"].(string))
	if err != nil {
		return nil, fmt.Errorf("invalid created_at format: %v", err)
	}

	return &pb.Message{
		Id:          raw["id"].(string),
		SenderId:    raw["sender_id"].(string),
		RecipientId: raw["recipient_id"].(string),
		Content:     raw["content"].(string),
		Read:        raw["read"].(bool),
		CreatedAt:   createdAt.Format(time.RFC3339),
	}, nil
}

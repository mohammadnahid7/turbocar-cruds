package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
	pb "wegugin/genproto/cruds"
	"wegugin/storage/postgres"
	"wegugin/storage/postgres/sqlc"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	zero "gopkg.in/guregu/null.v4/zero"
)

type CarService struct {
	pb.UnimplementedCrudsServiceServer
	logger *slog.Logger
	store  postgres.Store
}

func NewService(store postgres.Store, logger *slog.Logger) *CarService {
	return &CarService{
		store:  store,
		logger: logger,
	}
}

// ---------------------- CREATE CAR ----------------------
func (s *CarService) CreateCar(ctx context.Context, req *pb.CreateCarRequest) (*pb.Car, error) {
	// 1. Authentifikatsiya
	userID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	ownerUUID, err := uuid.Parse(userID)
	if err != nil {
		s.logger.Error("invalid owner ID format", "error", err)
		return nil, status.Error(codes.InvalidArgument, "invalid owner ID")
	}

	// 3. Narxni konvertatsiya qilish
	price, err := convertFloatToNumeric(req.GetPrice())
	if err != nil {
		s.logger.Error("invalid price format", "error", err)
		return nil, status.Error(codes.InvalidArgument, "invalid price value")
	}

	// 4. SQL parametrlari
	arg := sqlc.CreateCarParams{
		Type:        zero.StringFrom(req.GetType()),
		Make:        zero.StringFrom(req.GetMake()),
		Model:       zero.StringFrom(req.GetModel()),
		Year:        pgtype.Int4{Int32: req.GetYear(), Valid: req.GetYear() != 0},
		Color:       zero.StringFrom(req.GetColor()),
		Mileage:     pgtype.Int4{Int32: req.GetMileage(), Valid: req.GetMileage() != 0},
		Price:       price,
		Description: zero.StringFrom(req.GetDescription()),
		OwnerID:     pgtype.UUID{Bytes: ownerUUID, Valid: true},
		Location:    zero.StringFrom(req.GetLocation()),
	}

	// 5. Ma'lumotlar bazasiga saqlash
	dbCar, err := s.store.CreateCar(ctx, arg)
	if err != nil {
		s.logger.Error("failed to create car", "error", err)
		return nil, status.Error(codes.Internal, "failed to create car")
	}

	// 6. Protobuf response
	return s.convertDBCarToProto(dbCar), nil
}

func (s *CarService) GetCarById(ctx context.Context, req *pb.Id) (*pb.Car, error) {
	carID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid car ID format")
	}

	dbCar, err := s.store.GetCarById(ctx, pgtype.UUID{Bytes: carID, Valid: true})
	if err != nil {
		s.logger.Error("car not found", "error", err)
		return nil, status.Error(codes.NotFound, "car not found")
	}

	return s.convertDBCarToProtoWithImages(dbCar), nil
}

func (s *CarService) ListCars(ctx context.Context, req *pb.ListCarsRequest) (*pb.ListCarsResponse, error) {
	// User ID validation
	var userID uuid.UUID
	if req.GetUserId() != "" {
		var err error
		userID, err = uuid.Parse(req.GetUserId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid user_id format")
		}
	}
	if req.GetMinPrice() > req.GetMaxPrice() && req.GetMaxPrice() != 0 {
		return nil, status.Error(codes.InvalidArgument, "max_price must be greater than min_price")
	}
	minPrice, err := s.convertPriceToNumeric(req.GetMinPrice())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid min_price: %v", err)
	}

	maxPrice, err := s.convertPriceToNumeric(req.GetMaxPrice())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid max_price: %v", err)
	}
	params := sqlc.ListCarsParams{
		Type:       zero.StringFrom(req.GetType()),
		Location:   zero.StringFrom(req.GetLocation()),
		PriceOrder: zero.StringFrom(req.GetPriceOrder()),
		MinPrice:   minPrice, // <-- Yangi qo'shilgan
		MaxPrice:   maxPrice, // <-- Yangi qo'shilgan
		Offset:     pgtype.Int4{Int32: req.GetOffset(), Valid: req.GetOffset() != 0},
		Limit:      pgtype.Int4{Int32: req.GetLimit(), Valid: req.GetLimit() != 0},
		UserID:     pgtype.UUID{Bytes: userID, Valid: req.GetUserId() != ""},
	}

	dbCars, err := s.store.ListCars(ctx, params)
	if err != nil {
		s.logger.Error("failed to list cars", "error", err)
		return nil, status.Error(codes.Internal, "failed to list cars")
	}

	cars := make([]*pb.Car, len(dbCars))
	for i, dbCar := range dbCars {
		cars[i] = s.convertListCarToProto(dbCar)
	}

	return &pb.ListCarsResponse{Cars: cars}, nil
}

func (s *CarService) UpdateCar(ctx context.Context, req *pb.UpdateCarRequest) (*pb.Empty, error) {
	// 1. Tegishlik tekshiruvi
	if err := s.checkCarOwnership(ctx, req.GetId()); err != nil {
		return nil, err
	}

	// 2. Narxni konvertatsiya qilish
	price, err := convertFloatToNumeric(req.GetPrice())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid price value")
	}

	// 3. SQL parametrlari
	carID, _ := uuid.Parse(req.GetId())
	arg := sqlc.UpdateCarParams{
		Type:        zero.StringFrom(req.GetType()),
		Make:        zero.StringFrom(req.GetMake()),
		Model:       zero.StringFrom(req.GetModel()),
		Year:        pgtype.Int4{Int32: req.GetYear(), Valid: req.GetYear() != 0},
		Color:       zero.StringFrom(req.GetColor()),
		Mileage:     pgtype.Int4{Int32: req.GetMileage(), Valid: req.GetMileage() != 0},
		Price:       price,
		Description: zero.StringFrom(req.GetDescription()),
		Available:   pgtype.Bool{Bool: req.GetAvailable(), Valid: true},
		Location:    zero.StringFrom(req.GetLocation()),
		ID:          pgtype.UUID{Bytes: carID, Valid: true},
	}

	// 4. Ma'lumotlarni yangilash
	if err := s.store.UpdateCar(ctx, arg); err != nil {
		s.logger.Error("failed to update car", "error", err)
		return nil, status.Error(codes.Internal, "failed to update car")
	}

	return &pb.Empty{}, nil
}

func (s *CarService) DeleteCar(ctx context.Context, req *pb.Id) (*pb.Empty, error) {
	// 1. Tegishlik tekshiruvi
	if err := s.checkCarOwnership(ctx, req.GetId()); err != nil {
		return nil, err
	}

	// 2. UUID konvertatsiyasi
	carID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid car ID format")
	}

	// 3. O'chirish
	if err := s.store.DeleteCar(ctx, pgtype.UUID{Bytes: carID, Valid: true}); err != nil {
		s.logger.Error("failed to delete car", "error", err)
		return nil, status.Error(codes.Internal, "failed to delete car")
	}

	return &pb.Empty{}, nil
}

func (s *CarService) IncrementCarReviewCount(ctx context.Context, req *pb.Id) (*pb.Empty, error) {
	carID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid car ID format")
	}

	// 3. O'chirish
	if err := s.store.IncrementCarReviewCount(ctx, pgtype.UUID{Bytes: carID, Valid: true}); err != nil {
		s.logger.Error("failed to increment car review count", "error", err)
		return nil, status.Error(codes.Internal, "failed to increment car review count")
	}

	return &pb.Empty{}, nil
}

func (s *CarService) CheckCarOwnership(ctx context.Context, req *pb.BoolCheckCar) (*pb.BoolCheck, error) {

	// 2. UUID konvertatsiyalari
	userUUID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}
	carUUID, err := uuid.Parse(req.GetCarId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid car ID format")
	}

	// 3. Tekshiruvni amalga oshirish
	isOwner, err := s.store.CheckCarOwnership(ctx, sqlc.CheckCarOwnershipParams{
		ID:      pgtype.UUID{Bytes: carUUID, Valid: true},
		OwnerID: pgtype.UUID{Bytes: userUUID, Valid: true},
	})
	if err != nil {
		s.logger.Error("ownership check failed", "error", err)
		return nil, status.Error(codes.Internal, "ownership check failed")
	}

	return &pb.BoolCheck{Result: isOwner}, nil
}

func (s *CarService) SearchCar(ctx context.Context, req *pb.SearchCarRequest) (*pb.ListCarsResponse, error) {
	params := sqlc.SearchCarParams{
		Query:  zero.StringFrom(req.GetQuery()),
		Offset: pgtype.Int8{Int64: int64(req.GetOffset()), Valid: req.GetOffset() != 0},
		Limit:  pgtype.Int8{Int64: int64(req.GetLimit()), Valid: req.GetLimit() != 0},
	}

	dbCars, err := s.store.SearchCar(ctx, params)
	if err != nil {
		s.logger.Error("failed to search cars", "error", err)
		return nil, status.Error(codes.Internal, "failed to search cars")
	}

	cars := make([]*pb.Car, len(dbCars))
	for i, dbCar := range dbCars {
		cars[i] = s.convertSearchCarToProto(dbCar)
	}

	return &pb.ListCarsResponse{Cars: cars}, nil
}

// ---------------------- SAVED CARS ----------------------

// SaveCar - Avtomobilni saqlash
func (s *CarService) SaveCar(ctx context.Context, req *pb.SaveCarRequest) (*pb.Empty, error) {
	userID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}
	carUUID, err := uuid.Parse(req.GetCarId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid car ID format")
	}

	// 3. Ma'lumotlar bazasiga saqlash
	_, err = s.store.CreateSavedCar(ctx, sqlc.CreateSavedCarParams{
		UserID: pgtype.UUID{Bytes: userUUID, Valid: true},
		CarID:  pgtype.UUID{Bytes: carUUID, Valid: true},
	})
	if err != nil {
		s.logger.Error("failed to save car", "error", err)
		return nil, status.Error(codes.Internal, "failed to save car")
	}

	return &pb.Empty{}, nil
}

func (s *CarService) GetSavedCarsByUser(ctx context.Context, req *pb.GetSavedCarsRequest) (*pb.ListSavedCarsResponse, error) {
	userUUID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	dbSavedCars, err := s.store.GetSavedCarsByUser(ctx, pgtype.UUID{Bytes: userUUID, Valid: true})
	if err != nil {
		s.logger.Error("failed to get saved cars", "error", err)
		return nil, status.Error(codes.Internal, "failed to retrieve saved cars")
	}

	savedCars := make([]*pb.SavedCar, 0, len(dbSavedCars))
	for _, sc := range dbSavedCars {
		// Timestamplarni stringga o'tkazamiz
		createdAtStr := sc.CreatedAt.Time.Format(time.RFC3339)
		updatedAtStr := sc.UpdatedAt.Time.Format(time.RFC3339)

		savedCars = append(savedCars, &pb.SavedCar{
			Id:        sc.ID,
			UserId:    sc.UserID,
			CarId:     sc.CarID,
			CreatedAt: createdAtStr,
			UpdatedAt: updatedAtStr,
		})
	}

	return &pb.ListSavedCarsResponse{SavedCars: savedCars}, nil
}

// DeleteSavedCar - Saqlangan avtomobilni o'chirish
func (s *CarService) DeleteSavedCar(ctx context.Context, req *pb.DeleteSavedCarRequest) (*pb.Empty, error) {
	// 1. Tegishlik tekshiruvi
	if err := s.checkSavedCarOwnership(ctx, req.GetId()); err != nil {
		return nil, err
	}

	// 2. UUID konvertatsiyasi
	savedCarUUID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid saved car ID format")
	}

	// 3. O'chirish
	if err := s.store.DeleteSavedCar(ctx, pgtype.UUID{Bytes: savedCarUUID, Valid: true}); err != nil {
		s.logger.Error("failed to delete saved car", "error", err)
		return nil, status.Error(codes.Internal, "failed to delete saved car")
	}

	return &pb.Empty{}, nil
}

// DeleteSavedCarsByCarId - Avtomobil ID bo'yicha saqlanganlarni o'chirish
func (s *CarService) DeleteSavedCarsByCarId(ctx context.Context, req *pb.CarId) (*pb.Empty, error) {
	// 1. UUID konvertatsiyasi
	carUUID, err := uuid.Parse(req.GetCarId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid car ID format")
	}

	// 2. O'chirish
	if err := s.store.DeleteSavedCarsByCarId(ctx, pgtype.UUID{Bytes: carUUID, Valid: true}); err != nil {
		s.logger.Error("failed to delete saved cars by car ID", "error", err)
		return nil, status.Error(codes.Internal, "failed to delete saved cars")
	}

	return &pb.Empty{}, nil
}

// CheckSavedCarOwnership - Saqlangan avtomobil tegishligini tekshirish
func (s *CarService) CheckSavedCarOwnership(ctx context.Context, req *pb.BoolCheckSavedCars) (*pb.BoolCheck, error) {
	// 1. UUID konvertatsiyalari
	userUUID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}
	savedCarUUID, err := uuid.Parse(req.GetSavedCarId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid saved car ID format")
	}

	// 2. Tekshiruv
	isOwner, err := s.store.CheckSavedCarOwnership(ctx, sqlc.CheckSavedCarOwnershipParams{
		ID:     pgtype.UUID{Bytes: savedCarUUID, Valid: true},
		UserID: pgtype.UUID{Bytes: userUUID, Valid: true},
	})
	if err != nil {
		s.logger.Error("saved car ownership check failed", "error", err)
		return nil, status.Error(codes.Internal, "ownership check failed")
	}

	return &pb.BoolCheck{Result: isOwner}, nil
}

// ---------------------- NOTIFICATIONS ----------------------

// CreateNotification - Yangi bildirishnoma yaratish
func (s *CarService) CreateNotification(ctx context.Context, req *pb.CreateNotificationRequest) (*pb.Empty, error) {
	// 2. UUID konvertatsiyasi
	userUUID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	// 1. Autentifikatsiya
	config := &firebase.Config{
		ProjectID: "wegugin-cars-notifications",
	}

	// Firebase ilovasini ishga tushirish
	opt := option.WithCredentialsFile("./firebase/wegugin-cars-notifications-firebase-adminsdk-fbsvc-0016bd1639.json")
	app, err := firebase.NewApp(context.Background(), config, opt)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Ilovani ishga tushirishda xatolik: %v", err))
		return nil, status.Error(codes.Internal, "failed to initialize Firebase app")
	}
	// FCM mijozini olish
	client, err := app.Messaging(context.Background())
	if err != nil {
		s.logger.Error(fmt.Sprintf("FCM mijozini olishda xatolik: %v", err))
		return nil, status.Error(codes.Internal, "failed to initialize FCM client")
	}
	resptoken, err := s.store.GetNotificationTokensByUserId(ctx, pgtype.UUID{Bytes: userUUID, Valid: true})
	if err != nil {
		s.logger.Error("Failed to get notification tokens", "error", err)
		return nil, status.Error(codes.Internal, "failed to retrieve notification tokens")
	}

	for _, v := range resptoken {
		// Xabar yaratish
		message := &messaging.Message{
			Notification: &messaging.Notification{
				Title: req.GetType(),
				Body:  req.GetMessage(),
			},
			Token: v.Token,
		}

		go func(token string) {
			_, err := client.Send(context.Background(), message)
			if err != nil {
				// Faqatgina xatolarni log qilamiz, lekin jarayonni to'xtatmaymiz
				s.logger.Error("FCM xabar yuborishda xato",
					"token", token,
					"error", err,
				)
			}
		}(v.Token)
	}

	// 3. SQL parametrlari
	arg := sqlc.CreateNotificationParams{
		UserID:  pgtype.UUID{Bytes: userUUID, Valid: true},
		Type:    zero.StringFrom(req.GetType()),
		Message: zero.StringFrom(req.GetMessage()),
	}

	// 4. Ma'lumotlar bazasiga yozish
	_, err = s.store.CreateNotification(ctx, arg)
	if err != nil {
		s.logger.Error("failed to create notification", "error", err)
		return nil, status.Error(codes.Internal, "failed to create notification")
	}

	return &pb.Empty{}, nil
}

// GetAllNotificationsByUserId - Foydalanuvchining barcha bildirishnomalari
func (s *CarService) GetAllNotificationsByUserId(ctx context.Context, req *pb.GetUnreadNotificationsRequest) (*pb.ListNotificationsResponse, error) {
	// 1. UUID konvertatsiyasi
	userUUID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	// 2. Ma'lumotlarni olish
	dbNotifications, err := s.store.GetNotificationsByUser(ctx, pgtype.UUID{Bytes: userUUID, Valid: true})
	if err != nil {
		s.logger.Error("failed to get notifications", "error", err)
		return nil, status.Error(codes.Internal, "failed to retrieve notifications")
	}

	// 3. Protobuf response
	notifications := make([]*pb.Notification, len(dbNotifications))
	for i, dbNotif := range dbNotifications {
		notifications[i] = s.convertDBNotificationToProto(dbNotif)
	}

	return &pb.ListNotificationsResponse{Notifications: notifications}, nil
}

// GetUnreadNotifications - O'qilmagan bildirishnomalar
func (s *CarService) GetUnreadNotifications(ctx context.Context, req *pb.GetUnreadNotificationsRequest) (*pb.ListNotificationsResponse, error) {
	// 1. UUID konvertatsiyasi
	userUUID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	// 2. Ma'lumotlarni olish
	dbNotifications, err := s.store.GetUnreadNotificationsByUser(ctx, pgtype.UUID{Bytes: userUUID, Valid: true})
	if err != nil {
		s.logger.Error("failed to get unread notifications", "error", err)
		return nil, status.Error(codes.Internal, "failed to retrieve notifications")
	}

	// 3. Protobuf response
	notifications := make([]*pb.Notification, len(dbNotifications))
	for i, dbNotif := range dbNotifications {
		notifications[i] = s.convertDBNotificationToProto(dbNotif)
	}

	return &pb.ListNotificationsResponse{Notifications: notifications}, nil
}

// MarkNotificationAsRead - Bildirishnomani o'qilgan deb belgilash
func (s *CarService) MarkNotificationAsRead(ctx context.Context, req *pb.MarkNotificationAsReadRequest) (*pb.Empty, error) {
	// 2. UUID konvertatsiyasi
	notifUUID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid notification ID format")
	}

	// 3. Yangilash
	if err := s.store.MarkNotificationAsRead(ctx, pgtype.UUID{Bytes: notifUUID, Valid: true}); err != nil {
		s.logger.Error("failed to mark notification as read", "error", err)
		return nil, status.Error(codes.Internal, "failed to update notification")
	}

	return &pb.Empty{}, nil
}

// DeleteNotification - Bildirishnomani o'chirish
func (s *CarService) DeleteNotification(ctx context.Context, req *pb.DeleteNotificationRequest) (*pb.Empty, error) {
	// 2. UUID konvertatsiyasi
	notifUUID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid notification ID format")
	}

	// 3. O'chirish
	if err := s.store.DeleteNotification(ctx, pgtype.UUID{Bytes: notifUUID, Valid: true}); err != nil {
		s.logger.Error("failed to delete notification", "error", err)
		return nil, status.Error(codes.Internal, "failed to delete notification")
	}

	return &pb.Empty{}, nil
}

// ---------------------- MESSAGES ----------------------

func (s *CarService) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.Message, error) {
	senderUUID, err := uuid.Parse(req.GetSenderId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid sender ID format")
	}
	recipientUUID, err := uuid.Parse(req.GetRecipientId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid recipient ID format")
	}

	// 3. Xabarni yaratish
	arg := sqlc.CreateMessageParams{
		SenderID:    pgtype.UUID{Bytes: senderUUID, Valid: true},
		RecipientID: pgtype.UUID{Bytes: recipientUUID, Valid: true},
		Content:     zero.StringFrom(req.GetContent()),
	}

	dbMessage, err := s.store.CreateMessage(ctx, arg)
	if err != nil {
		s.logger.Error("failed to send message", "error", err)
		return nil, status.Error(codes.Internal, "failed to send message")
	}

	return s.convertDBMessageToProto(dbMessage), nil
}

func (s *CarService) GetMessagesByUser(ctx context.Context, req *pb.GetMessagesByUserRequest) (*pb.ListMessagesResponse, error) {
	userUUID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID")
	}

	// Bazadan ma'lumotlarni olish
	dbGroups, err := s.store.GetMessagesByUser(ctx, pgtype.UUID{Bytes: userUUID, Valid: true})
	if err != nil {
		s.logger.Error("failed to get messages", "error", err)
		return nil, status.Error(codes.Internal, "failed to retrieve messages")
	}

	// JSON ma'lumotlarni konvertatsiya qilish
	response := &pb.ListMessagesResponse{Groups: make([]*pb.ListMessagesResponsewithUserID, 0)}

	for _, dbGroup := range dbGroups {
		group := &pb.ListMessagesResponsewithUserID{
			UserId: uuid.Must(uuid.FromBytes(dbGroup.UserID.Bytes[:])).String(),
		}

		// JSON ma'lumotlarni parse qilish
		var rawMessages []map[string]interface{}
		if err := json.Unmarshal(dbGroup.Messages, &rawMessages); err != nil {
			s.logger.Warn("failed to parse message JSON", "error", err)
			continue
		}

		// Har bir xabarni konvertatsiya qilish
		for _, rawMsg := range rawMessages {
			msg, err := s.convertRawMessage(rawMsg)
			if err != nil {
				s.logger.Warn("failed to convert message", "error", err)
				continue
			}
			group.Messages = append(group.Messages, msg)
		}

		response.Groups = append(response.Groups, group)
	}
	return response, nil
}

func (s *CarService) GetMessageByUserAndId(ctx context.Context, req *pb.GetMessageByUserAndIdReq) (*pb.GetMessageByUserAndIdRes, error) {
	// IDlarni validate qilish
	user1UUID, err := uuid.Parse(req.GetFirstUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user 1 ID")
	}
	user2UUID, err := uuid.Parse(req.GetSecondUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user 2 ID")
	}

	// Message'larni bazadan olish
	dbMessages, err := s.store.GetMessagesByUserAndId(ctx, sqlc.GetMessagesByUserAndIdParams{
		User1ID: pgtype.UUID{Bytes: user1UUID, Valid: true},
		User2ID: pgtype.UUID{Bytes: user2UUID, Valid: true},
	})
	if err != nil {
		s.logger.Error("failed to get messages", "error", err)
		return nil, status.Error(codes.Internal, "failed to retrieve messages")
	}

	// Message'larni protobuf formatiga o'tkazish
	pbMessages := make([]*pb.Message, 0, len(dbMessages))
	for _, dbMsg := range dbMessages {
		createdAtStr := ""
		if dbMsg.CreatedAt.Valid {
			createdAtStr = dbMsg.CreatedAt.Time.Format(time.RFC3339)
		}
		pbMsg := &pb.Message{
			Id:          dbMsg.ID,
			SenderId:    dbMsg.SenderID,
			RecipientId: dbMsg.RecipientID,
			Content:     dbMsg.Content,
			Read:        dbMsg.Read.Bool,
			CreatedAt:   createdAtStr,
		}
		pbMessages = append(pbMessages, pbMsg)
	}

	// Protobuf response tayyorlash
	return &pb.GetMessageByUserAndIdRes{
		UserId:   req.GetSecondUserId(),
		Messages: pbMessages,
	}, nil
}

// MarkMessageAsRead - Xabarni o'qilgan deb belgilash
func (s *CarService) MarkMessageAsRead(ctx context.Context, req *pb.MessageId) (*pb.Empty, error) {
	// 2. UUID konvertatsiyasi
	msgUUID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid message ID format")
	}

	// 3. Yangilash
	if err := s.store.MarkMessageAsRead(ctx, pgtype.UUID{Bytes: msgUUID, Valid: true}); err != nil {
		s.logger.Error("failed to mark message as read", "error", err)
		return nil, status.Error(codes.Internal, "failed to update message")
	}

	return &pb.Empty{}, nil
}

// DeleteMessage - Xabarni o'chirish
func (s *CarService) DeleteMessage(ctx context.Context, req *pb.DeleteMessageRequest) (*pb.Empty, error) {
	// 2. UUID konvertatsiyasi
	msgUUID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid message ID format")
	}

	// 3. O'chirish
	if err := s.store.DeleteMessage(ctx, pgtype.UUID{Bytes: msgUUID, Valid: true}); err != nil {
		s.logger.Error("failed to delete message", "error", err)
		return nil, status.Error(codes.Internal, "failed to delete message")
	}

	return &pb.Empty{}, nil
}

// CheckMessageOwnership - Xabar tegishligini tekshirish
func (s *CarService) CheckMessageOwnership(ctx context.Context, req *pb.BoolCheckMessage) (*pb.BoolCheck, error) {
	// 1. UUID konvertatsiyalari
	userUUID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}
	msgUUID, err := uuid.Parse(req.GetMessageId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid message ID format")
	}

	// 2. Tekshiruv
	isOwner, err := s.store.CheckMessageOwnership(ctx, sqlc.CheckMessageOwnershipParams{
		ID:     pgtype.UUID{Bytes: msgUUID, Valid: true},
		UserID: pgtype.UUID{Bytes: userUUID, Valid: true},
	})
	if err != nil {
		s.logger.Error("message ownership check failed", "error", err)
		return nil, status.Error(codes.Internal, "ownership check failed")
	}

	return &pb.BoolCheck{Result: isOwner}, nil
}

// ---------------------- NOTIFICATION TOKENS ----------------------

// RegisterNotificationToken - Yangi FCM tokenni ro'yxatdan o'tkazish
func (s *CarService) RegisterNotificationToken(ctx context.Context, req *pb.RegisterNotificationTokenRequest) (*pb.Empty, error) {
	userID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	// 2. UUID konvertatsiyasi
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	// 3. Platformani tekshirish
	allowedPlatforms := map[string]bool{"andr": true, "ios": true, "web": true}
	if !allowedPlatforms[req.GetPlatform()] {
		return nil, status.Error(codes.InvalidArgument, "invalid platform")
	}

	// 4. SQL parametrlari
	arg := sqlc.CreateNotificationTokenParams{
		UserID:   pgtype.UUID{Bytes: userUUID, Valid: true},
		Token:    zero.StringFrom(req.GetToken()),
		Platform: zero.StringFrom(req.Platform),
	}

	// 5. Bazaga yozish
	_, err = s.store.CreateNotificationToken(ctx, arg)
	if err != nil {
		s.logger.Error("failed to register token", "error", err)
		return nil, status.Error(codes.Internal, "failed to register token")
	}

	return &pb.Empty{}, nil
}

// GetNotificationTokensByUserId - Foydalanuvchi tokenlarini olish
func (s *CarService) GetNotificationTokensByUserId(ctx context.Context, req *pb.GetNotificationTokensByUserIdRequest) (*pb.ListNotificationTokensResponse, error) {
	userUUID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	// 3. Ma'lumotlarni olish
	dbTokens, err := s.store.GetNotificationTokensByUserId(ctx, pgtype.UUID{Bytes: userUUID, Valid: true})
	if err != nil {
		s.logger.Error("failed to get tokens", "error", err)
		return nil, status.Error(codes.Internal, "failed to retrieve tokens")
	}

	// 4. Protobuf konvertatsiyasi
	tokens := make([]*pb.NotificationToken, len(dbTokens))
	for i, dbToken := range dbTokens {
		tokens[i] = &pb.NotificationToken{
			Id:        dbToken.ID,
			UserId:    dbToken.UserID,
			Token:     dbToken.Token,
			Platform:  fmt.Sprintf("%v", dbToken.Platform), // interface{} ni stringga o'tkazish
			CreatedAt: dbToken.CreatedAt.Time.Format(time.RFC3339),
			UpdatedAt: dbToken.UpdatedAt.Time.Format(time.RFC3339),
		}
	}

	return &pb.ListNotificationTokensResponse{Tokens: tokens}, nil
}

// DeleteNotificationToken - Tokenni o'chirish
func (s *CarService) DeleteNotificationToken(ctx context.Context, req *pb.DeleteNotificationTokenRequest) (*pb.Empty, error) {
	tokenUUID, err := uuid.Parse(req.GetTokenId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid token ID format")
	}

	if err := s.store.DeleteNotificationToken(ctx, pgtype.UUID{Bytes: tokenUUID, Valid: true}); err != nil {
		s.logger.Error("failed to delete token", "error", err)
		return nil, status.Error(codes.Internal, "failed to delete token")
	}

	return &pb.Empty{}, nil
}

// ---------------------- IMAGES ----------------------

func (s *CarService) AddImage(ctx context.Context, req *pb.AddImageRequest) (*pb.Image, error) {
	carUUID, err := uuid.Parse(req.GetCarId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid car ID format")
	}

	arg := sqlc.AddImageParams{
		CarID:    pgtype.UUID{Bytes: carUUID, Valid: true},
		Filename: zero.StringFrom(req.GetFilename()),
	}

	dbImage, err := s.store.AddImage(ctx, arg)
	if err != nil {
		s.logger.Error("failed to add image", "error", err)
		return nil, status.Error(codes.Internal, "failed to upload image")
	}

	return &pb.Image{
		Id:         dbImage.ID,
		CarId:      dbImage.CarID,
		Filename:   dbImage.Filename,
		UploadedAt: dbImage.UploadedAt.Time.Format(time.RFC3339),
	}, nil
}

// GetImagesByCar - Avtomobil rasmlarini olish
func (s *CarService) GetImagesByCar(ctx context.Context, req *pb.CarId) (*pb.ListImagesResponse, error) {
	// 1. UUID konvertatsiyasi
	carUUID, err := uuid.Parse(req.GetCarId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid car ID format")
	}

	// 2. Ma'lumotlarni olish
	dbImages, err := s.store.GetImagesByCar(ctx, pgtype.UUID{Bytes: carUUID, Valid: true})
	if err != nil {
		s.logger.Error("failed to get images", "error", err)
		return nil, status.Error(codes.Internal, "failed to retrieve images")
	}

	// 3. Konvertatsiya
	images := make([]*pb.Image, len(dbImages))
	for i, dbImg := range dbImages {
		images[i] = &pb.Image{
			Id:         dbImg.ID,
			CarId:      dbImg.CarID,
			Filename:   dbImg.Filename,
			UploadedAt: dbImg.UploadedAt.Time.Format(time.RFC3339),
		}
	}

	return &pb.ListImagesResponse{Images: images}, nil
}

// DeleteImage - Rasmni o'chirish
func (s *CarService) DeleteImage(ctx context.Context, req *pb.ImageId) (*pb.Empty, error) {
	// 1. Rasmni bazadan olish
	imageUUID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid image ID format")
	}

	// 2. O'chirish
	if err := s.store.DeleteImage(ctx, pgtype.UUID{Bytes: imageUUID, Valid: true}); err != nil {
		s.logger.Error("failed to delete image", "error", err)
		return nil, status.Error(codes.Internal, "failed to delete image")
	}

	return &pb.Empty{}, nil
}

// DeleteImagesByCarId - Avtomobil rasmlarini o'chirish
func (s *CarService) DeleteImagesByCarId(ctx context.Context, req *pb.CarId) (*pb.Empty, error) {
	carUUID, err := uuid.Parse(req.GetCarId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid car ID format")
	}

	if err := s.store.DeleteImagesByCarId(ctx, pgtype.UUID{Bytes: carUUID, Valid: true}); err != nil {
		s.logger.Error("failed to delete images", "error", err)
		return nil, status.Error(codes.Internal, "failed to delete images")
	}

	return &pb.Empty{}, nil
}

// GetImagesByCar - Avtomobil rasmlarini olish
func (s *CarService) GetImageByID(ctx context.Context, req *pb.ImageId) (*pb.Image, error) {
	// 1. UUID konvertatsiyasi
	imageID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid image ID format")
	}

	// 2. Ma'lumotlarni olish
	dbImage, err := s.store.GetImageById(ctx, pgtype.UUID{Bytes: imageID, Valid: true})
	if err != nil {
		s.logger.Error("failed to get images", "error", err)
		return nil, status.Error(codes.Internal, "failed to retrieve images")
	}

	// 3. Konvertatsiya
	return &pb.Image{
		Id:         dbImage.ID,
		CarId:      dbImage.CarID,
		Filename:   dbImage.Filename,
		UploadedAt: dbImage.UploadedAt.Time.Format(time.RFC3339),
	}, nil
}

// ---------------------- COMMENTS ----------------------

// CreateComment - Kommentariya yaratish
func (s *CarService) CreateComment(ctx context.Context, req *pb.CreateCommentRequest) (*pb.Comment, error) {
	// 1. Autentifikatsiya
	userID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// 2. UUID konvertatsiyalari
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}

	carUUID, err := uuid.Parse(req.GetCarId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid car ID format")
	}

	// 3. SQL parametrlari
	arg := sqlc.CreateCommentParams{
		UserID:  pgtype.UUID{Bytes: userUUID, Valid: true},
		CarID:   pgtype.UUID{Bytes: carUUID, Valid: true},
		Content: zero.StringFrom(req.GetContent()),
	}

	// 4. Bazaga yozish
	dbComment, err := s.store.CreateComment(ctx, arg)
	if err != nil {
		s.logger.Error("failed to create comment", "error", err)
		return nil, status.Error(codes.Internal, "failed to create comment")
	}

	// 5. Protobuf response
	return &pb.Comment{
		Id:        dbComment.ID,
		UserId:    dbComment.UserID,
		CarId:     dbComment.CarID,
		Content:   dbComment.Content,
		CreatedAt: dbComment.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt: dbComment.UpdatedAt.Time.Format(time.RFC3339),
	}, nil
}

// GetCommentsByCar - Avtomobil kommentariyalari
func (s *CarService) GetCommentsByCar(ctx context.Context, req *pb.CarId) (*pb.ListCommentsResponse, error) {
	// 1. UUID konvertatsiyasi
	carUUID, err := uuid.Parse(req.GetCarId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid car ID format")
	}

	// 2. Ma'lumotlarni olish
	dbComments, err := s.store.GetCommentsByCar(ctx, pgtype.UUID{Bytes: carUUID, Valid: true})
	if err != nil {
		s.logger.Error("failed to get comments", "error", err)
		return nil, status.Error(codes.Internal, "failed to retrieve comments")
	}

	// 3. Konvertatsiya
	comments := make([]*pb.Comment, len(dbComments))
	for i, dbComment := range dbComments {
		comments[i] = &pb.Comment{
			Id:        dbComment.ID,
			UserId:    dbComment.UserID,
			CarId:     dbComment.CarID,
			Content:   dbComment.Content,
			CreatedAt: dbComment.CreatedAt.Time.Format(time.RFC3339),
			UpdatedAt: dbComment.UpdatedAt.Time.Format(time.RFC3339),
		}
	}

	return &pb.ListCommentsResponse{Comments: comments}, nil
}

// UpdateComment - Kommentariyani yangilash
func (s *CarService) UpdateComment(ctx context.Context, req *pb.UpdateCommentRequest) (*pb.Empty, error) {
	// 1. Tegishlik tekshiruvi
	if err := s.checkCommentOwnership(ctx, req.GetId()); err != nil {
		return nil, err
	}

	// 2. UUID konvertatsiyasi
	commentUUID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid comment ID format")
	}

	// 3. Yangilash
	arg := sqlc.UpdateCommentParams{
		Content: zero.StringFrom(req.GetContent()),
		ID:      pgtype.UUID{Bytes: commentUUID, Valid: true},
	}

	if err := s.store.UpdateComment(ctx, arg); err != nil {
		s.logger.Error("failed to update comment", "error", err)
		return nil, status.Error(codes.Internal, "failed to update comment")
	}

	return &pb.Empty{}, nil
}

// DeleteComment - Kommentariyani o'chirish
func (s *CarService) DeleteComment(ctx context.Context, req *pb.CommentId) (*pb.Empty, error) {
	// 1. Tegishlik tekshiruvi
	if err := s.checkCommentOwnership(ctx, req.GetId()); err != nil {
		return nil, err
	}

	// 2. UUID konvertatsiyasi
	commentUUID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid comment ID format")
	}

	// 3. O'chirish
	if err := s.store.DeleteComment(ctx, pgtype.UUID{Bytes: commentUUID, Valid: true}); err != nil {
		s.logger.Error("failed to delete comment", "error", err)
		return nil, status.Error(codes.Internal, "failed to delete comment")
	}

	return &pb.Empty{}, nil
}

// DeleteCommentsByCarId - Avtomobil kommentariyalarini o'chirish
func (s *CarService) DeleteCommentsByCarId(ctx context.Context, req *pb.CarId) (*pb.Empty, error) {
	// 1. Tegishlik tekshiruvi
	if err := s.checkCarOwnership(ctx, req.GetCarId()); err != nil {
		return nil, err
	}

	// 2. UUID konvertatsiyasi
	carUUID, err := uuid.Parse(req.GetCarId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid car ID format")
	}

	// 3. O'chirish
	if err := s.store.DeleteCommentsByCarId(ctx, pgtype.UUID{Bytes: carUUID, Valid: true}); err != nil {
		s.logger.Error("failed to delete comments", "error", err)
		return nil, status.Error(codes.Internal, "failed to delete comments")
	}

	return &pb.Empty{}, nil
}

// CheckCommentOwnership - Kommentariya tegishligini tekshirish
func (s *CarService) CheckCommentOwnership(ctx context.Context, req *pb.BoolCheckComment) (*pb.BoolCheck, error) {
	// 1. UUID konvertatsiyalari
	userUUID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID format")
	}
	commentUUID, err := uuid.Parse(req.GetCommentId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid comment ID format")
	}

	// 2. Tekshiruv
	isOwner, err := s.store.CheckCommentOwnership(ctx, sqlc.CheckCommentOwnershipParams{
		ID:     pgtype.UUID{Bytes: commentUUID, Valid: true},
		UserID: pgtype.UUID{Bytes: userUUID, Valid: true},
	})
	if err != nil {
		s.logger.Error("comment ownership check failed", "error", err)
		return nil, status.Error(codes.Internal, "ownership check failed")
	}

	return &pb.BoolCheck{Result: isOwner}, nil
}

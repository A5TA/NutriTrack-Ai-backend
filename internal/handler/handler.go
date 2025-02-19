package handler

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/A5TA/NutriTrack-Ai-backend/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GetAllMeals fetches meals between startDate and endDate for a specific user
func GetAllMeals(c *gin.Context) {
	// Extract userId, startDate and endDate from the form data
	userId := c.PostForm("userId")
	if userId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		log.Println("User ID is required")
		return
	}

	startDateStr := c.PostForm("startDate")
	endDateStr := c.PostForm("endDate")

	// Load EST timezone
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load EST timezone"})
		log.Println("Failed to load EST timezone:", err)
		return
	}

	// Parse startDate and set it to EST
	startDate, err := time.ParseInLocation("2006-01-02", startDateStr, loc)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid startDate format, expected YYYY-MM-DD"})
		log.Println("Invalid startDate format, expected YYYY-MM-DD")
		return
	}
	// Parse endDate, or default it to startDate if not provided
	var endDate time.Time
	if endDateStr == "" {
		endDate = startDate
	} else {
		endDate, err = time.ParseInLocation("2006-01-02", endDateStr, loc)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid endDate format, expected YYYY-MM-DD"})
			log.Println("Invalid endDate format, expected YYYY-MM-DD")
			return
		}
	}

	// Get the MongoDB collection
	collection := config.GetCollection("meals")

	// Create the filter for the query
	filter := bson.M{
		"userId": userId,
		"date": bson.M{
			"$gte": startDate,
			"$lt":  endDate.Add(24 * time.Hour),
		},
	}

	// Find the meals in the date range for the user
	cursor, err := collection.Find(context.TODO(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch meals"})
		log.Println("Failed to fetch meals")
		return
	}
	defer cursor.Close(context.TODO())

	// Decode the meals
	var meals []Meal
	if err = cursor.All(context.TODO(), &meals); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode meals"})
		log.Println("Failed to decode meals")
		return
	}

	// Return the meals
	c.JSON(http.StatusOK, gin.H{
		"meals": meals,
		"count": len(meals),
	})
}

func GetMeal(c *gin.Context) {
	//using the userId and meals id to get the meal from the database
	userId := c.PostForm("userId")
	if userId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}
	mealID := c.PostForm("mealId")
	if mealID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Meal ID is required for searching meals"})
		return
	}

	// Convert mealID to ObjectID
	objectID, err := primitive.ObjectIDFromHex(mealID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Meal ID format"})
		return
	}
	// Get the MongoDB collection
	collection := config.GetCollection("meals")

	// Find the meal by ID
	var meal Meal
	err = collection.FindOne(context.TODO(), bson.M{"_id": objectID}).Decode(&meal)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Meal not found"})
		return
	}

	// Return the meal
	c.JSON(http.StatusOK, meal)
}

func UpdateMeal(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}

func DeleteMeal(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}

// StorePrediction handles the food prediction storing request
func StorePrediction(c *gin.Context) {
	//Get the users Id
	userId := c.PostForm("userId")
	if userId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required for storing predictions"})
		return
	}

	//Get the image's predicted name
	name := c.PostForm("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name is required for storing predictions"})
		return
	}

	//Get the image's predicted name
	mealType := c.PostForm("mealtype")
	if mealType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Meal Type is required for storing predictions"})
		return
	}

	// Get the image file from the request
	file, _, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to get image attachment"})
		return
	}
	defer file.Close()

	// Read the image bytes
	imgBytes, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read image"})
		return
	}

	// Validate the image format by reading the file header
	contentType := http.DetectContentType(imgBytes[:512])
	log.Printf("Detected content type: %s", contentType)

	// Explicitly decode as JPEG or PNG
	var img image.Image
	switch contentType {
	case "image/jpeg":
		img, err = jpeg.Decode(bytes.NewReader(imgBytes))
	case "image/png":
		img, err = png.Decode(bytes.NewReader(imgBytes))
	default:
		err = fmt.Errorf("unsupported content type: %s", contentType)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to decode image: %v", err)})
		return
	}

	// Store the image in the temp folder for now - replace with db later
	document, err := storeImage(img, name, mealType, userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Prediction failed: %v", err)})
		return
	}

	//return the created meal object
	c.JSON(http.StatusOK, gin.H{"msg": name + " image was stored successfully!", "meal": document})
}

func storeImage(img image.Image, name string, mealType string, userId string) (primitive.M, error) {
	// Get the MongoDB collection
	collection := config.GetCollection("meals")

	// Create a temporary directory to store the image if not made already
	tmpDir := "images" //Change to Amazon S3 bucket in future
	err := os.MkdirAll(tmpDir, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %v", err)
	}

	// Save the image to a temporary file (JPEG format)
	imgUUID := uuid.New() //Generate a unique ID for the image
	fileName := name + mealType + imgUUID.String() + "_image_*.jpg"
	tmpfile, err := os.CreateTemp(tmpDir, fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer tmpfile.Close()

	// Encode the image as JPEG and save it to the temporary file
	err = jpeg.Encode(tmpfile, img, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to encode image as JPEG: %v", err)
	}

	// Get current EST time
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, fmt.Errorf("failed to load EST location: %v", err)
	}
	currentTimeEST := time.Now().In(loc)

	// Get the macros for the meal
	// Get the MongoDB collection for macros
	macrosCollection := config.GetCollection("macros")

	// Find the macros by meal name
	var macros Macros
	err = macrosCollection.FindOne(context.TODO(), bson.M{"name": name}).Decode(&macros)
	if err != nil {
		return nil, fmt.Errorf("failed to find macros for meal %s: %v", name, err)
	}

	// Create the document to insert
	document := bson.M{
		"name":     name,
		"mealType": mealType,
		"image":    tmpfile.Name()[7:], // Remove the "images/" prefix from the image file name
		"userId":   userId,
		"macros":   macros,
		"date":     currentTimeEST,
	}

	// Insert the document into the collection
	res, err := collection.InsertOne(context.TODO(), document)
	if err != nil {
		return nil, fmt.Errorf("failed to store image %s: %v", name, err)
	}

	// Log the inserted ID for reference
	log.Printf("Image %s stored successfully with ID: %v", name, res.InsertedID)

	// Add the inserted ID to the document
	document["_id"] = res.InsertedID

	return document, nil
}

// Getting Macros for a meal
func GetMealMacros(c *gin.Context) {
	//using only the meals name to find the macros using
	mealName := c.Param("mealName")
	if mealName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Meal Name is required for searching meals"})
		return
	}

	// Get the MongoDB collection
	collection := config.GetCollection("macros")

	// Find the meal by ID
	var macros Macros
	err := collection.FindOne(context.TODO(), bson.M{"name": mealName}).Decode(&macros)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Meal not found"})
		return
	}

	// Return the meal
	c.JSON(http.StatusOK, macros)
}

func GetAllMealMacros(c *gin.Context) {
	// Get the MongoDB collection
	collection := config.GetCollection("macros")

	// Find all the meals
	cursor, err := collection.Find(context.TODO(), bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch meals"})
		return
	}
	defer cursor.Close(context.TODO())

	// Decode the meals
	var macros []Macros
	if err = cursor.All(context.TODO(), &macros); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode meals"})
		return
	}

	// Return the meals
	c.JSON(http.StatusOK, gin.H{
		"meals": macros,
		"count": len(macros),
	})
}

// AddMealMacros adds the macros for a meal
func AddMealMacros(c *gin.Context) {
	// Get the meal name
	mealName := c.PostForm("mealName")
	if mealName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Meal Name is required"})
		return
	}

	// Get the macros
	calories := c.PostForm("calories")
	protein := c.PostForm("protein")
	carbs := c.PostForm("carbs")
	fat := c.PostForm("fat")

	// Convert the macros to float64
	caloriesFloat, err := strconv.ParseFloat(calories, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid calories format"})
		return
	}
	proteinFloat, err := strconv.ParseFloat(protein, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid protein format"})
		return
	}
	carbsFloat, err := strconv.ParseFloat(carbs, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid carbs format"})
		return
	}
	fatFloat, err := strconv.ParseFloat(fat, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid fat format"})
		return
	}

	// Get the MongoDB collection
	collection := config.GetCollection("macros")

	// Create the document to insert
	document := bson.M{
		"name":      mealName,
		"calories":  caloriesFloat,
		"protein":   proteinFloat,
		"carbs":     carbsFloat,
		"fat":       fatFloat,
		"createdAt": time.Now(),
	}

	// Insert the document into the collection
	_, err = collection.InsertOne(context.TODO(), document)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store macros"})
		return
	}

	// Return the inserted document
	c.JSON(http.StatusOK, document)
}

func AddBulkMealMacros(c *gin.Context) {
	var requestData map[string]map[string]float64

	// Parse the JSON body
	if err := c.BindJSON(&requestData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON format"})
		return
	}

	// Convert input data into an array of Macro documents
	var macros []interface{}
	for meal, data := range requestData {
		macros = append(macros, Macro{
			Name:     meal,
			Calories: data["calories"],
			Protein:  data["protein"],
			Carbs:    data["carbs"],
			Fat:      data["fat"],
			CreatedAt:  time.Now(),
		})
	}

	// fmt.Print(macros)
	// Get the MongoDB collection
	collection := config.GetCollection("macros")

	// Insert into MongoDB
	_, err := collection.InsertMany(context.TODO(), macros)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store macros"})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{"message": "Bulk macros uploaded successfully", "count": len(macros)})
}

// Get image from the images folder by name
func GetImage(c *gin.Context) {
	// Get the image name from the URL
	imageName := c.Param("imageName")
	if imageName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Image name is required"})
		return
	}

	log.Printf("Getting image: %s", imageName)

	// Open the image file
	imgFile, err := os.Open("images/" + imageName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		return
	}
	defer imgFile.Close()

	// Set the content type based on the image extension
	contentType := "image/jpeg"
	if imageName[len(imageName)-4:] == ".png" {
		contentType = "image/png"
	}

	// Set the headers and return the image
	c.Header("Content-Type", contentType)
	http.ServeContent(c.Writer, c.Request, imageName, time.Now(), imgFile)
}

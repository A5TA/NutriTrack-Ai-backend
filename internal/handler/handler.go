package handler

import (
	// "io"
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/A5TA/NutriTrack-Ai-backend/internal/ai"
	"github.com/gin-gonic/gin"
)

func PostMeal(c *gin.Context) {
	var newMeal Meal

	if err := c.BindJSON(&newMeal); err != nil {
		return
	}

	mockMeals = append(mockMeals, newMeal)
	c.JSON(http.StatusOK, newMeal)
}

// GetAllMeals fetches meals between startDate and endDate
func GetAllMeals(c *gin.Context) {
	// Extract startDate and endDate from URL parameters
	startDateStr := c.Param("startDate")
	endDateStr := c.Param("endDate")

	// Parse dates
	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid startDate format, expected YYYY-MM-DD"})
		return
	}
	// Parse endDate, or default it to startDate if not provided
	var endDate time.Time
	if endDateStr == "" {
		endDate = startDate
	} else {
		endDate, err = time.Parse("2006-01-02", endDateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid endDate format, expected YYYY-MM-DD"})
			return
		}
	}

	// Filter meals by date range
	var filteredMeals []Meal
	for _, meal := range mockMeals {
		if meal.TimeEaten.After(startDate) && meal.TimeEaten.Before(endDate.Add(24*time.Hour)) {
			filteredMeals = append(filteredMeals, meal)
		}
	}

	// Return the filtered meals
	c.JSON(http.StatusOK, filteredMeals)
}

func GetMeal(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}

func UpdateMeal(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}

func DeleteMeal(c *gin.Context) {
	c.Status(http.StatusNotImplemented)
}

// PredictFood handles the food prediction request
func PredictFood(c *gin.Context) {
	// Get the image file from the request
	file, _, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to get image"})
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

	// Get prediction from the predictor
	result, err := ai.PredictFood(img)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Prediction failed: %v", err)})
		return
	}

	// Return the prediction
	c.JSON(http.StatusOK, gin.H{"prediction": result})
}

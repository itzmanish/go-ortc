package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

var defaultLogger Logger

type loggerStruct struct {
	log *logrus.Entry
}

type Logger interface {
	Info(msg ...any)
	Infof(format string, msg ...any)
	Debug(msg ...any)
	Debugf(format string, msg ...any)
	Warn(msg ...any)
	Warnf(format string, msg ...any)
	Error(msg ...any)
	Errorf(format string, msg ...any)

	WithField(key string, value interface{}) Logger
}

func init() {
	// Log as JSON instead of the default ASCII formatter.
	// logrus.SetFormatter(&logrus.JSONFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	logrus.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	logrus.SetLevel(logrus.InfoLevel)
	defaultLogger = NewLogger("sfu")
}

func NewLogger(name string) Logger {
	logr := logrus.WithField("name", name)
	return &loggerStruct{
		log: logr,
	}
}

func (logger *loggerStruct) WithField(key string, value interface{}) Logger {
	return &loggerStruct{
		log: logger.log.WithField(key, value),
	}
}

func (logger *loggerStruct) Info(msg ...any) {
	logger.log.Infoln(msg...)
}

func (logger *loggerStruct) Debug(msg ...any) {
	logger.log.Debugln(msg...)
}

func (logger *loggerStruct) Warn(msg ...any) {
	logger.log.Warnln(msg...)
}

func (logger *loggerStruct) Error(msg ...any) {
	logger.log.Errorln(msg...)
}

func (logger *loggerStruct) Infof(format string, msg ...any) {
	logger.log.Infof(format, msg...)
}

func (logger *loggerStruct) Debugf(format string, msg ...any) {
	logger.log.Debugf(format, msg...)
}

func (logger *loggerStruct) Warnf(format string, msg ...any) {
	logger.log.Warnf(format, msg...)
}

func (logger *loggerStruct) Errorf(format string, msg ...any) {
	logger.log.Errorf(format, msg...)
}

func Info(msg ...any) {
	defaultLogger.Info(msg...)
}

func Debug(msg ...any) {
	defaultLogger.Debug(msg...)
}

func Warn(msg ...any) {
	defaultLogger.Warn(msg...)
}

func Error(msg ...any) {
	defaultLogger.Error(msg...)
}

func Infof(format string, msg ...any) {
	defaultLogger.Infof(format, msg...)
}

func Debugf(format string, msg ...any) {
	defaultLogger.Debugf(format, msg...)
}

func Warnf(format string, msg ...any) {
	defaultLogger.Warnf(format, msg...)
}

func Errorf(format string, msg ...any) {
	defaultLogger.Errorf(format, msg...)
}

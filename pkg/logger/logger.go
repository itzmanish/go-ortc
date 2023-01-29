package logger

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

var defaultLogger Logger

type loggerStruct struct {
	log *logrus.Entry
	logrus.Level
}

type Option struct {
	logrus.Level
	logrus.Fields
}

type Options func(*Option)

type Logger interface {
	Info(msg ...any)
	Infof(format string, msg ...any)
	Debug(msg ...any)
	Debugf(format string, msg ...any)
	Warn(msg ...any)
	Warnf(format string, msg ...any)
	Error(msg ...any)
	Errorf(format string, msg ...any)
	Trace(msg ...any)
	Tracef(format string, msg ...any)

	WithField(key string, value interface{}) Logger
}

func init() {
	defaultLogger = NewLogger("sfu", WithLevel("info"))
}

func NewLogger(name string, opts ...Options) Logger {
	options := Option{
		Level: logrus.InfoLevel,
	}
	for _, o := range opts {
		o(&options)
	}

	logr := logrus.New()
	logr.SetLevel(options.Level)
	logr.SetOutput(os.Stdout)
	return &loggerStruct{
		log: logr.WithField("name", name),
	}
}

func WithLevel(level string) Options {
	return func(o *Option) {
		l, e := logrus.ParseLevel(level)
		if e != nil {
			fmt.Println("provided level is not a valid level", level)
		}
		o.Level = l
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

func (logger *loggerStruct) Trace(msg ...any) {
	logger.log.Traceln(msg...)
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

func (logger *loggerStruct) Tracef(format string, msg ...any) {
	logger.log.Tracef(format, msg...)
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

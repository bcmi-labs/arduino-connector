#!/bin/bash

go build

scp arduino-connector.sh admin@downloads-01.arduino.cc:/var/www/files/tools/arduino-connector.sh
scp arduino-connector.sh admin@downloads-02.arduino.cc:/var/www/files/tools/arduino-connector.sh
scp arduino-connector.sh admin@downloads-04.arduino.cc:/var/www/files/tools/arduino-connector.sh

scp arduino-connector admin@downloads-01.arduino.cc:/var/www/files/tools/arduino-connector
scp arduino-connector admin@downloads-02.arduino.cc:/var/www/files/tools/arduino-connector
scp arduino-connector admin@downloads-04.arduino.cc:/var/www/files/tools/arduino-connector

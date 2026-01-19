#!/usr/bin/env python3

import qrcode

def main():
    data_to_encode = 'https://allisonrosememorialfund.org/'
    filename = 'img/qr.png'
    img = qrcode.make(data_to_encode)
    img.save(filename)

    print(f"QR code generated and saved as {filename}")

if __name__ == '__main__':
    main()

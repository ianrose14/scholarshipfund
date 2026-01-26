#!/usr/bin/env python3

import pymupdf

# references
# * https://pymupdf.readthedocs.io/en/latest/widget.html

def_fontsize = 14
margin = 30
def_font = 'Helvetica'

def insert_centered_text(text, page, y, xmax, fontname=def_font, fontsize=def_fontsize, underline=False, color=(0, 0, 0)):
    text_length = pymupdf.get_text_length(text, fontname=fontname, fontsize=fontsize)
    x = (xmax - text_length) / 2
    h = line_height(pymupdf.Font(fontname), fontsize)
    page.insert_text((x, y+h), text, fontname=fontname, fontsize=fontsize, color=color)
    if underline:
        page.draw_line((x, y+h+8), (x + text_length, y+h+8), color=color, width=1)
        return y + h + 1
    return y + h

def create_application_form(output_path="forms/flier.pdf"):
    doc = pymupdf.open()  # Create a new PDF document
    page = doc.new_page() # Add a new page

    r = page.bound()
    xmax = r[2] - margin
    ymax = r[3] - margin

    side_len = 80
    img_rect = pymupdf.Rect(margin/2, margin, margin/2 + side_len, margin + side_len)
    page.insert_image(img_rect, filename="img/stethoscope.png")

    side_len = 80
    img_rect = pymupdf.Rect(xmax - margin/2 - side_len, margin, xmax - margin/2, margin + side_len)
    page.insert_image(img_rect, filename="img/mortarboard.png")

    text = 'Ready to Elevate'
    ypos = insert_centered_text(text, page, margin, xmax, fontsize=def_fontsize+24)

    text = 'Your Career?'
    ypos = insert_centered_text(text, page, ypos, xmax, fontsize=def_fontsize+24)

    ypos += 60

    lines = [
        'If you\'re a NICU nurse, NNP school may be',
        'closer than you think...',
        '',
        'Scholarships are available now through',
        'the Dr Allison Rose Memorial Fund.',
    ]

    for i, line in enumerate(lines):
        ypos = insert_centered_text(line, page, ypos, xmax, fontsize=def_fontsize+10)

    side_len = 250
    qr_x0 = xmax/2 - side_len/2  # center horizontally
    qr_y0 = ymax*0.65 - side_len/2  # 65% down the page

    img_rect = pymupdf.Rect(qr_x0, qr_y0, qr_x0 + side_len, qr_y0 + side_len)
    page.insert_image(img_rect, filename="img/qr.png")

    text = 'Scan for eligibility & application details'
    ypos = insert_centered_text(text, page, qr_y0 + side_len - 10, xmax, fontsize=def_fontsize+6)

    text = 'https://www.allisonrosememorialfund.org/'
    ypos = insert_centered_text(text, page, ypos, xmax, fontsize=def_fontsize+6)

    ypos += 40

    text = 'Dr. Allison Rose Memorial Fund, Inc.'
    ypos = insert_centered_text(text, page, ypos, xmax, fontsize=def_fontsize-2, color=(0.4, 0.4, 0.4))

    text = '501(c)(3) nonprofit organization'
    ypos = insert_centered_text(text, page, ypos, xmax, fontsize=def_fontsize-2, color=(0.4, 0.4, 0.4))

    doc.save(output_path)
    doc.close()
    print("Wrote", output_path)

def line_height(font, fontsize):
    h = font.ascender - font.descender # Relative to fontsize 1
    return h * fontsize

def main():
    create_application_form()

if __name__ == "__main__":
    main()

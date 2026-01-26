#!/usr/bin/env python3

import pymupdf

# --------------------
# Global layout config
# --------------------
def_font = 'Helvetica'
def_fontsize = 10
title_fontsize = 16
margin = 20
line_gap = 6

# --------------------
# Utility helpers
# --------------------

def insert_centered_text(text, page, y, xmax, fontname=def_font, fontsize=def_fontsize, underline=False, color=(0, 0, 0)):
    text_length = pymupdf.get_text_length(text, fontname=fontname, fontsize=fontsize)
    x = (xmax - text_length) / 2
    h = line_height(pymupdf.Font(fontname), fontsize)
    page.insert_text((x, y+h), text, fontname=fontname, fontsize=fontsize, color=color)
    if underline:
        page.draw_line((x, y+h+8), (x + text_length, y+h+8), color=color, width=1)
        return y + h + 1
    return y + h

def line_height(font, fontsize):
    h = font.ascender - font.descender # Relative to fontsize 1
    return h * fontsize

def draw_label_and_form_field(page, xpos, ypos, label, field_width=200, signature=False, gap=6, fontname=def_font, fontsize=def_fontsize):
    h = line_height(pymupdf.Font(fontname), fontsize)
    label_len = pymupdf.get_text_length(label, fontname=fontname, fontsize=fontsize)
    page.insert_text((xpos, ypos), label, fontname=fontname, fontsize=fontsize)

    if not signature:
        widget = pymupdf.Widget()
        widget.rect = pymupdf.Rect(xpos + label_len + gap, ypos - 12, xpos + label_len + gap + field_width, ypos + 5)
        widget.field_name = label
        widget.field_type = pymupdf.PDF_WIDGET_TYPE_TEXT
        widget.text_font = fontname
        widget.text_fontsize = fontsize
        page.add_widget(widget)

    page.draw_line(
        (xpos + label_len + gap, ypos + 4),
        (xpos + label_len + gap + field_width, ypos + 4),
        color=(0.75, 0.75, 0.75),
        width=1
    )

    return ypos + h + line_gap

def draw_checkbox(page, xpos, ypos, label, name, fontsize=def_fontsize):
    h = line_height(pymupdf.Font(def_font), fontsize)
    page.insert_text(
        (xpos + h + 5, ypos),
        label,
        fontname=def_font,
        fontsize=fontsize
    )

    widget = pymupdf.Widget()
    widget.rect = pymupdf.Rect(xpos, ypos - h + 3, xpos + h, ypos + 3)
    widget.field_name = name
    widget.field_type = pymupdf.PDF_WIDGET_TYPE_CHECKBOX
    widget.text_font = 'ZaDb'
    widget.text_fontsize = 0
    widget.border_color = (0, 0, 0)
    widget.border_width = 1
    page.add_widget(widget)
    return ypos + h + line_gap

# --------------------
# Main generator
# --------------------
def main(output_path="forms/financial_aid_certification_v1.pdf"):
    doc = pymupdf.open()
    page = doc.new_page()

    r = page.bound()
    xmax = r[2] - margin
    ymax = r[3] - margin
    img_rect = pymupdf.Rect(margin, margin, 60, 60)
    page.insert_image(img_rect, filename="img/ar_logo_128.png")

    ypos = margin

    def insert_fund_name():
        fontsize = def_fontsize
        h = line_height(pymupdf.Font(def_font), fontsize)

        text = 'Dr. Allison Thomas Rose'
        text_length = pymupdf.get_text_length(text, fontname=def_font, fontsize=fontsize)
        page.insert_text((xmax - text_length, h + margin), text, fontname=def_font, fontsize=fontsize)

        text = 'Memorial Fund'
        text_length = pymupdf.get_text_length(text, fontname=def_font, fontsize=fontsize)
        page.insert_text((xmax - text_length, 2*h + margin), text, fontname=def_font, fontsize=fontsize)

        text = 'A 501(c)(3) nonprofit organization'
        text_length = pymupdf.get_text_length(text, fontname=def_font, fontsize=fontsize-2)
        page.insert_text((xmax - text_length, 3*h + margin), text, fontname=def_font, fontsize=fontsize-2)

    insert_fund_name()

    # --------------------
    # Header
    # --------------------
    title = "Financial Aid Certification"
    insert_centered_text(title, page, ypos, xmax, fontsize=def_fontsize+8, underline=True)

    ypos = 85

    intro = (
        "This form is required for nursing students to apply for funding through the "
        "Allison Rose Memorial Fund scholarship program. The information the school of "
        "nursing provides is strictly confidential and only used to verify cost of attendance."
    )
    page.insert_textbox(
        (margin, ypos, xmax, ypos + 60),
        intro,
        fontname=def_font,
        fontsize=def_fontsize
    )
    ypos += 60

    # --------------------
    # Student section
    # --------------------
    ypos = draw_label_and_form_field(page, margin, ypos, "Student Name:", 260, fontname='Helvetica-Bold')
    ypos += 5
    text = "Student Signature to release information:"
    ypos = draw_label_and_form_field(page, margin, ypos, text, 200, signature=True, fontname='Helvetica-Bold')
    page.draw_line((margin, ypos), (xmax, ypos), width=3)
    ypos += 10

    # --------------------
    # Administrator section
    # --------------------
    page.insert_text(
        (margin, ypos + line_height(pymupdf.Font(def_font), def_fontsize)),
        "To be completed by Financial Aid Administrator Only",
        fontname='Helvetica-Bold',
        fontsize=def_fontsize
    )
    ypos += 20

    text = "Please provide us with the most current information available at the school of nursing.  Completed forms may be emailed to applications@allisonrosememorialfund.org"
    page.insert_textbox(
        (margin, ypos, xmax, ypos + 60),
        text,
        fontname=def_font,
        fontsize=def_fontsize
    )
    ypos += 60

    draw_checkbox(page, margin + 300, ypos, "Estimated", "estimated")
    draw_checkbox(page, margin + 400, ypos, "Actual", "actual")
    ypos = draw_label_and_form_field(page, margin, ypos, "Total Cost of Attendance $", 120, gap=2)
    ypos = draw_label_and_form_field(page, margin, ypos, 'For which academic year?', 120)
    draw_label_and_form_field(page, margin, ypos, "Tuition / Fees $", 120, gap=2)
    ypos = draw_label_and_form_field(page, margin + 260, ypos, "Books $", 120, gap=2)
    draw_label_and_form_field(page, margin, ypos, "Loan Fees $", 120, gap=2)
    ypos = draw_label_and_form_field(page, margin + 260, ypos, "Room & Board $", 120, gap=2)

    ypos += 10
    text = "1. What is the per credit tuition rate for 2024-2025 at your school?  $"
    ypos = draw_label_and_form_field(page, margin, ypos, text, 150, gap=2)

    page.insert_text(
        (margin, ypos),
        "2. Has the student completed a FAFSA form?",
        fontname=def_font,
        fontsize=def_fontsize
    )
    draw_checkbox(page, margin + 220, ypos, "Yes", "fafsa_yes")
    draw_checkbox(page, margin + 280, ypos, "No", "fafsa_no")
    ypos += 20

    ypos = draw_label_and_form_field(
        page, margin, ypos,
        "3. Student Aid Index (SAI) from FAFSA",
        120, gap=2
    )

    ypos = draw_label_and_form_field(page, margin, ypos, "4. Student ID#", 200)

    ypos = draw_label_and_form_field(
        page, margin, ypos,
        "5. Cumulative GPA (4.0 scale)",
        200
    )

    # Citizenship
    page.insert_text((margin, ypos), "6. Is the student a U.S. citizen or eligible non-citizen (per FAFSA)?", fontname=def_font, fontsize=def_fontsize)
    draw_checkbox(page, margin + 310, ypos, "Yes", "citizen_yes")
    ypos = draw_checkbox(page, margin + 370, ypos, "No", "citizen_no")

    # --------------------
    # Administrator signature
    # --------------------
    ypos += 30
    draw_label_and_form_field(page, margin, ypos, "FAA Name", 240)
    ypos = draw_label_and_form_field(page, 350, ypos, "Title", 100)
    ypos = draw_label_and_form_field(page, margin, ypos, "E-Mail", 280)
    ypos = draw_label_and_form_field(page, margin, ypos, "Phone / Ext #", 200)
    ypos = draw_label_and_form_field(page, margin, ypos, "School", 400)
    draw_label_and_form_field(page, margin, ypos, "Signature", 300, signature=True)
    ypos = draw_label_and_form_field(page, 400, ypos, "Date", 100)

    # --------------------
    # Bursar's Office
    # --------------------
    ypos += 20
    text = "If this student is awarded a scholarship, checks are sent to the financial aid or bursar's office for deposit in the student's tuition account. Please indicate the mailing address where the check is to be mailed:"
    page.insert_textbox(
        (margin, ypos, xmax, ypos + 40),
        text,
        fontname=def_font,
        fontsize=def_fontsize
    )
    ypos += 50
    ypos = draw_label_and_form_field(page, margin, ypos, "Send to attention of:", 300)
    ypos = draw_label_and_form_field(page, margin, ypos, "Mailing Address", 380)
    draw_label_and_form_field(page, margin, ypos, "City", 200)
    draw_label_and_form_field(page, margin + 250, ypos, "State", 80)
    ypos = draw_label_and_form_field(page, margin + 380, ypos, "Zip", 80)

    # --------------------
    # Footer
    # --------------------
    ypos += 10
    text = "Thank you for completing this form!"
    insert_centered_text(text, page, ypos, xmax, fontname='Helvetica-Oblique', fontsize=def_fontsize + 2)

    text = 'applications@allisonrosememorialfund.org'
    h = line_height(pymupdf.Font(def_font), def_fontsize - 2)
    insert_centered_text(text, page, ymax - 2*h, xmax, fontsize=def_fontsize - 2)

    text = 'Dr. Allison Thomas Rose Memorial Fund | allisonrosememorialfund.org'
    h = line_height(pymupdf.Font(def_font), def_fontsize - 2)
    insert_centered_text(text, page, ymax - h, xmax, fontsize=def_fontsize - 2)

    doc.save(output_path)
    doc.close()
    print("Wrote", output_path)

# --------------------
if __name__ == "__main__":
    main()

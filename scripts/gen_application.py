#!/usr/bin/env python3

import pymupdf

# references
# * https://pymupdf.readthedocs.io/en/latest/widget.html

def_fontsize = 10
margin = 20
def_font = 'Helvetica'

def create_application_form(output_path="forms/application_form_v1.pdf"):
    doc = pymupdf.open()  # Create a new PDF document
    page = doc.new_page() # Add a new page

    r = page.bound()
    xmax = r[2] - margin
    ymax = r[3] - margin
    img_rect = pymupdf.Rect(margin, margin, 60, 60)
    page.insert_image(img_rect, filename="img/rose2.png")

    def insert_fund_name():
        fontsize = def_fontsize
        h = line_height(pymupdf.Font(def_font), fontsize)

        text = 'Dr. Allison Rose'
        text_length = pymupdf.get_text_length(text, fontname=def_font, fontsize=fontsize)
        page.insert_text((xmax - text_length, h + margin), text, fontname=def_font, fontsize=fontsize)

        text = 'Memorial Fund'
        text_length = pymupdf.get_text_length(text, fontname=def_font, fontsize=fontsize)
        page.insert_text((xmax - text_length, 2*h + margin), text, fontname=def_font, fontsize=fontsize)

        text = 'A 501(c)(3) nonprofit organization'
        text_length = pymupdf.get_text_length(text, fontname=def_font, fontsize=fontsize-2)
        page.insert_text((xmax - text_length, 3*h + margin), text, fontname=def_font, fontsize=fontsize-2)

    insert_fund_name()

    def insert_centered_text(y, text, fontname=def_font, fontsize=def_fontsize, underline=False):
        text_length = pymupdf.get_text_length(text, fontname=fontname, fontsize=fontsize)
        x = (xmax - text_length) / 2
        h = line_height(pymupdf.Font(fontname), fontsize)
        page.insert_text((x, y+h), text, fontname=fontname, fontsize=fontsize)
        if underline:
            page.draw_line((x, y+h+8), (x + text_length, y+h+8), color=(0, 0, 0), width=1)
            return y + h + 1
        return y + h

    title = 'Application Form'
    insert_centered_text(margin, title, fontname='Helvetica-Bold', fontsize=def_fontsize+8, underline=True)

    ypos = 90

    lines = [
        "  1. Please complete all sections of this form.  Typed responses are preferred.",
        "  2. Sign and date the form at the bottom.",
        "  3. Visit https://www.allisonrosememorialfund.org/apply.html to submit and view next steps.",
        "  4. Questions?  Please e-mail applications@allisonrosememorialfund.org"
    ]

    for i, line in enumerate(lines):
        page.insert_text((margin, ypos + 15 * i), line, fontname=def_font, fontsize=def_fontsize)

    ypos = ypos + 10 + 15 * len(lines)

    page.draw_line((margin, ypos), (xmax, ypos), color=(.6, .6, .6), width=1)
    txt = "  I. Personal Information"
    page.insert_text((margin, ypos+15), txt, fontname=def_font, fontsize=def_fontsize + 2)
    page.draw_line((margin, ypos+22), (xmax, ypos+22), color=(.6, .6, .6), width=1)
    ypos += 45

    rows = [
        [
            {"name": "fullname_field", "label": "Name (last, first)", "size": -1},
        ],
        [
            {"name": "addr_field", "label": "Address", "size": -1},
        ],
        [
            {"name": "addr2_field", "label": "Address (line 2)", "size": -1},
        ],
        [
            {"name": "city_field", "label": "City", "size": 180},
            {"name": "state_field", "label": "State", "size": 85},
            {"name": "zip_field", "label": "Zip Code", "size": -1},
        ],
        [
            {"name": "phone_field", "label": "Phone", "size": 180},
            {"name": "email_field", "label": "Email", "size": -1},
        ]
    ]

    ypos = insert_form_fields(rows, page, ypos, xalign=True)

    page.draw_line((margin, ypos), (xmax, ypos), color=(.6, .6, .6), width=1)
    txt = "  II. Academic Information"
    page.insert_text((margin, ypos+15), txt, fontname=def_font, fontsize=def_fontsize + 2)
    page.draw_line((margin, ypos+22), (xmax, ypos+22), color=(.6, .6, .6), width=1)
    ypos += 45

    rows = [
        [{"label": "NNP degree-granting program(s) you currently attend or are applying to for admission:"}],
        [{"name": "program_field", "size": -1}],
        [{"label": "Please check exactly one (1) of the following:"}],
        [{"name": "current_fulltime_check", "label": "I am already enrolled in the FULL-time NNP program listed above.", "size": 20, "type": pymupdf.PDF_WIDGET_TYPE_CHECKBOX, "reverse": True}],
        [{"name": "current_parttime_check", "label": "I am already enrolled in the PART-time NNP program listed above.", "size": 20, "type": pymupdf.PDF_WIDGET_TYPE_CHECKBOX, "reverse": True}],
        [{"name": "applying_fulltime_check", "label": "I am currently applying to the FULL-time NNP program(s) listed above.", "size": 20, "type": pymupdf.PDF_WIDGET_TYPE_CHECKBOX, "reverse": True}],
        [{"name": "applying_parttime_check", "label": "I am currently applying to the PART-time NNP program(s) listed above.", "size": 20, "type": pymupdf.PDF_WIDGET_TYPE_CHECKBOX, "reverse": True}],
        [{"name": "applying_other_check", "label": "Other (please explain below):", "size": 20, "type": pymupdf.PDF_WIDGET_TYPE_CHECKBOX, "reverse": True}],
        [{"name": "applying_other_field1", "size": -1}],
        [{"name": "applying_other_field2", "size": -1}],
        [],
        [
            {"name": "start_date_field", "label": "Intended start date", "size": 100},
            {"label": "          Planned enrollment status:"},
            {"name": "fulltime_field", "label": "Full-time", "size": 20, "type": pymupdf.PDF_WIDGET_TYPE_CHECKBOX, "reverse": True},
            {"name": "parttime_field", "label": "Part-time", "size": 20, "type": pymupdf.PDF_WIDGET_TYPE_CHECKBOX, "reverse": True},
        ],
        [
            {"label": "If part-time, please list your anticipated course of study, including credit-hours per semester."},
        ],
        [
            {"name": "parttime_details_year1_field", "size": -1},
        ],
        [
            {"name": "parttime_details_year2_field", "size": -1},
        ],
        [
            {"name": "parttime_details_year3_field", "size": -1},
        ]
    ]
    ypos = insert_form_fields(rows, page, ypos)

    ypos += 20

    page.draw_line((margin, ypos), (xmax, ypos), color=(.6, .6, .6), width=1)
    ypos += 18

    text = 'Applicant Certification'
    page.insert_text((margin, ypos), text, fontname=def_font, fontsize=def_fontsize+2)
    ypos += 20

    text = 'I certify that the information provided in this application is accurate and complete to the best of my knowledge,'
    page.insert_text((margin + 10, ypos), text, fontname=def_font, fontsize=def_fontsize)
    ypos += 13
    text = 'and I understand that providing false information may affect my eligibility for this scholarship.'
    page.insert_text((margin + 10, ypos), text, fontname=def_font, fontsize=def_fontsize)
    ypos += 35

    text = 'Applicant Signature:'
    xpos = margin + 10
    page.insert_text((xpos, ypos), text, fontname=def_font, fontsize=def_fontsize)
    text_length = pymupdf.get_text_length(text, fontname=def_font, fontsize=def_fontsize)
    xpos += text_length + 5
    page.draw_line((xpos, ypos+1), (xpos + 250, ypos+1), color=(0, 0, 0), width=1)
    xpos += 250 + 10
    text = 'Date:'
    page.insert_text((xpos, ypos), text, fontname=def_font, fontsize=def_fontsize)
    text_length = pymupdf.get_text_length(text, fontname=def_font, fontsize=def_fontsize)
    xpos += text_length + 5
    page.draw_line((xpos, ypos+1), (xpos + 90, ypos+1), color=(0, 0, 0), width=1)

    text = 'applications@allisonrosememorialfund.org'
    h = line_height(pymupdf.Font(def_font), def_fontsize - 2)
    insert_centered_text(ymax - 2*h, text, fontsize=def_fontsize - 2)

    text = 'Dr. Allison Rose Memorial Fund - https://allisonrosememorialfund.org'
    h = line_height(pymupdf.Font(def_font), def_fontsize - 2)
    insert_centered_text(ymax - h, text, fontsize=def_fontsize - 2)

    doc.save(output_path)
    doc.close()
    print("Wrote", output_path)

def insert_form_fields(rows, page, ypos, xalign=False):
    r = page.bound()
    xmax = r[2]
    def full_width(x0):
        return xmax - x0 - margin

    def txtlen(s):
        return pymupdf.get_text_length(s, fontname=def_font, fontsize=def_fontsize)

    largest_first_key = max([txtlen(row[0].get("label", "")) for row in rows if len(row) > 0])

    for row in rows:
        xpos = margin
        for i, field in enumerate(row):
            label = field.get("label", "")
            reverse = field.get("reverse", False)
            label_len = txtlen(label)
            if i == 0 and xalign and txtlen(label) < largest_first_key:
                label_len = largest_first_key
            if label and field.get("name") and not reverse and not (label[-1] in [",", ":", ";", "?", "!", "."]):
                label += ":"
                label_len += 1
            if label and not reverse:
                page.insert_text((xpos, ypos), label, fontname=def_font, fontsize=def_fontsize)
                xpos += label_len + 6
            if field.get("name"):
                widget = pymupdf.Widget()
                if field.get("size", -1) == -1:
                    field["size"] = full_width(xpos)

                widget.rect = pymupdf.Rect(xpos, ypos - 12, xpos + field["size"], ypos + 5)
                widget.field_name = field["name"]
                widget.field_type = field.get("type", pymupdf.PDF_WIDGET_TYPE_TEXT)
                if widget.field_type == pymupdf.PDF_WIDGET_TYPE_CHECKBOX:
                    widget.text_font = 'ZaDb'
                    widget.text_fontsize = 0
                    widget.border_color = (0, 0, 0)
                    widget.border_width = 1
                else:
                    widget.text_font = def_font
                    widget.text_fontsize = def_fontsize
                page.add_widget(widget)

                if widget.field_type != pymupdf.PDF_WIDGET_TYPE_CHECKBOX:
                    page.draw_line(
                        (xpos, ypos + 4),
                        (xpos + field["size"], ypos + 4),
                        color=(0.75, 0.75, 0.75),
                        width=1
                    )

                xpos += field["size"] + 4
            if label and reverse:
                page.insert_text((xpos, ypos), label, fontname=def_font, fontsize=def_fontsize)
                xpos += label_len + 6

            xpos += 20  # 20 horizontal padding in between fields
        ypos += 20

    return ypos

def line_height(font, fontsize):
    h = font.ascender - font.descender # Relative to fontsize 1
    return h * fontsize

def main():
    create_application_form()

if __name__ == "__main__":
    main()

"""Arma la plantilla genérica de Logday para el Excel de horas extra
(specs/plantillas/design.md §3).

Uso: python build_logday_xlsx.py <carpeta>. Deja en la carpeta la plantilla
con marcadores (`logday-horas-extra.xlsx`) y un libro lleno a mano, como lo
dejaría el motor (`logday-horas-extra-ejemplo.xlsx`), para ver cómo se ve.
Necesita openpyxl.
"""
import sys
from datetime import date, time

from openpyxl import Workbook
from openpyxl.styles import Alignment, Border, Font, PatternFill, Side
from openpyxl.worksheet.page import PageMargins

INK = "18181B"        # texto
MUTED = "71717A"      # texto secundario
ACCENT = "4F46E5"     # índigo de Logday
HEAD_FILL = "EEF2FF"  # encabezados
GROUP_FILL = "E0E7FF" # grupos de columnas
LABEL_FILL = "F4F4F5" # etiquetas de la cabecera
LINE = "D4D4D8"

thin = Side(style="thin", color=LINE)
BOX = Border(left=thin, right=thin, top=thin, bottom=thin)
TOTAL_BORDER = Border(left=thin, right=thin, top=Side(style="medium", color=ACCENT), bottom=thin)

FONT = "Calibri"
f_body = Font(name=FONT, size=11, color=INK)
f_head = Font(name=FONT, size=11, bold=True, color="312E81")
f_label = Font(name=FONT, size=11, bold=True, color=INK)
f_note = Font(name=FONT, size=9, italic=True, color=MUTED)

CENTER = Alignment(horizontal="center", vertical="center", wrap_text=True)
LEFT = Alignment(horizontal="left", vertical="center", wrap_text=True)

FMT_DATE = "dd/mm/yyyy"
FMT_TIME = "h:mm AM/PM"
FMT_NUM = '0.00;-0.00;"–"'  # el cero se ve como una raya

HEADERS = [
    "Fecha", "Solicitada por", "Actividad", "Observaciones",
    "Inicio", "Fin", "Total",
    "Diurnas", "Nocturnas", "Diurnas festivas", "Nocturnas festivas",
]
WIDTHS = [12, 20, 42, 24, 11, 11, 9, 10, 11, 12, 13]
ROW_MARKERS = [
    "{{fila.fecha}}", "{{fila.solicitada_por}}", "{{fila.actividad}}", "{{fila.observaciones}}",
    "{{fila.hora_inicio}}", "{{fila.hora_final}}", None,
    "{{fila.diurnas}}", "{{fila.nocturnas}}", "{{fila.diurnas_festivas}}", "{{fila.nocturnas_festivas}}",
]
ROW_FORMATS = [FMT_DATE, None, None, None, FMT_TIME, FMT_TIME, FMT_NUM, FMT_NUM, FMT_NUM, FMT_NUM, FMT_NUM]

# Ejemplo: noviembre de 2026, tres registros.
EXAMPLE_META = {"mes": "Noviembre 2026", "colaborador": "Ana Gómez", "cedula": "1.098.765.432"}
EXAMPLE_ROWS = [
    (date(2026, 11, 3), "Carlos Ruiz", "Despliegue de la versión 2.4 del portal de citas", "Compensatorio", time(18, 0), time(21, 30), 1.0, 2.5, 0, 0),
    (date(2026, 11, 9), "Carlos Ruiz", "Soporte al cierre contable del mes", "Pago", time(8, 0), time(14, 0), 0, 0, 6.0, 0),
    (date(2026, 11, 19), "Laura Pérez", "Migración de la base de datos de inventario", "Pago", time(19, 0), time(23, 0), 0, 4.0, 0, 0),
]

DATA_ROW = 8


def build(example: bool) -> Workbook:
    wb = Workbook()
    ws = wb.active
    ws.title = "Horas extra"
    ws.sheet_view.showGridLines = False

    for i, w in enumerate(WIDTHS):
        ws.column_dimensions[chr(65 + i)].width = w

    meta = EXAMPLE_META if example else {k: "{{%s}}" % k for k in EXAMPLE_META}

    # Título y cabecera.
    ws["A1"] = f"Horas extra — {meta['mes']}"
    ws["A1"].font = Font(name=FONT, size=16, bold=True, color=INK)
    ws.row_dimensions[1].height = 26
    for row, (label, key) in enumerate([("Nombre", "colaborador"), ("Documento", "cedula")], start=3):
        a = ws.cell(row=row, column=1, value=label)
        a.font, a.fill, a.border, a.alignment = f_label, PatternFill("solid", fgColor=LABEL_FILL), BOX, LEFT
        ws.merge_cells(start_row=row, start_column=2, end_row=row, end_column=4)
        b = ws.cell(row=row, column=2, value=meta[key])
        b.font, b.alignment = f_body, LEFT
        for c in range(2, 5):
            ws.cell(row=row, column=c).border = BOX

    # Grupos de columnas (fila 6) y encabezados (fila 7).
    for first, last, text in [(5, 7, "Horario"), (8, 11, "Horas extra")]:
        ws.merge_cells(start_row=6, start_column=first, end_row=6, end_column=last)
        cell = ws.cell(row=6, column=first, value=text)
        cell.font, cell.alignment = f_head, CENTER
        for c in range(first, last + 1):
            ws.cell(row=6, column=c).fill = PatternFill("solid", fgColor=GROUP_FILL)
            ws.cell(row=6, column=c).border = BOX
    for c, text in enumerate(HEADERS, start=1):
        cell = ws.cell(row=7, column=c, value=text)
        cell.font, cell.alignment, cell.border = f_head, CENTER, BOX
        cell.fill = PatternFill("solid", fgColor=HEAD_FILL)
    ws.row_dimensions[7].height = 32

    # Fila de datos: la plantilla tiene una; el ejemplo, una por registro.
    rows = EXAMPLE_ROWS if example else [None]
    for i, data in enumerate(rows):
        r = DATA_ROW + i
        for c in range(1, 12):
            cell = ws.cell(row=r, column=c)
            if c == 7:
                cell.value = f"=SUM(H{r}:K{r})"
            elif data is None:
                cell.value = ROW_MARKERS[c - 1]
            else:
                cell.value = data[c - 1 if c < 7 else c - 2]
            cell.font, cell.border = f_body, BOX
            cell.alignment = LEFT if c in (2, 3, 4) else CENTER
            if ROW_FORMATS[c - 1]:
                cell.number_format = ROW_FORMATS[c - 1]
    last_data = DATA_ROW + len(rows) - 1

    # Totales.
    tot = last_data + 1
    label = ws.cell(row=tot, column=6, value="Total")
    label.font = f_label
    label.alignment = Alignment(horizontal="right", vertical="center")
    for c in range(1, 12):
        ws.cell(row=tot, column=c).border = TOTAL_BORDER
    for c in range(7, 12):
        col = chr(64 + c)
        cell = ws.cell(row=tot, column=c, value=f"=SUM({col}{DATA_ROW}:{col}{last_data})")
        cell.font, cell.alignment, cell.number_format = f_label, CENTER, FMT_NUM
    ws.row_dimensions[tot].height = 20

    # Nota con las franjas (ley colombiana; editable).
    note = tot + 2
    ws.merge_cells(start_row=note, start_column=1, end_row=note, end_column=11)
    n = ws.cell(row=note, column=1, value=(
        "Diurnas: de 6:00 a. m. a 7:00 p. m.   ·   Nocturnas: de 7:00 p. m. a 6:00 a. m.   ·   "
        "Festivas: domingos y festivos.   ·   Observaciones: compensatorio o pago."
    ))
    n.font, n.alignment = f_note, Alignment(horizontal="left", vertical="center")

    # Vista e impresión.
    ws.freeze_panes = f"A{DATA_ROW}"
    ws.print_title_rows = "6:7"
    ws.page_setup.orientation = "landscape"
    ws.page_setup.fitToWidth = 1
    ws.page_setup.fitToHeight = 0
    ws.sheet_properties.pageSetUpPr.fitToPage = True
    ws.page_margins = PageMargins(left=0.4, right=0.4, top=0.5, bottom=0.5)
    wb.properties.title = "Horas extra"
    wb.properties.creator = "Logday"
    return wb


if __name__ == "__main__":
    out_dir = sys.argv[1]
    build(False).save(f"{out_dir}/logday-horas-extra.xlsx")
    build(True).save(f"{out_dir}/logday-horas-extra-ejemplo.xlsx")

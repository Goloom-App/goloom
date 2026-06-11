from app.scheduling.slots import format_schedule_label


def test_format_schedule_label_german():
    label = format_schedule_label("2026-06-13T20:00:00Z", language="de")
    assert "Samstag" in label
    assert "13.06.2026" in label


def test_format_schedule_label_english():
    label = format_schedule_label("2026-06-13T20:00:00Z", language="en")
    assert "Sat" in label
    assert "2026-06-13" in label

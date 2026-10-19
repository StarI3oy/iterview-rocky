import 'package:flutter/material.dart';
import 'package:flutter_code_editor/flutter_code_editor.dart';
import 'package:flutter_highlight/themes/monokai-sublime.dart';
import 'package:highlight/languages/go.dart';
import 'package:highlight/languages/sql.dart';

import '../theme.dart';

class PrepCodeEditor extends StatefulWidget {
  const PrepCodeEditor({
    super.key,
    required this.initialText,
    required this.language,
    this.readOnly = false,
    this.expands = false,
    this.onChanged,
  });

  final String initialText;
  final String language;
  final bool readOnly;
  final bool expands;
  final ValueChanged<String>? onChanged;

  @override
  State<PrepCodeEditor> createState() => _PrepCodeEditorState();
}

class _PrepCodeEditorState extends State<PrepCodeEditor> {
  late final CodeController _controller;

  @override
  void initState() {
    super.initState();
    _controller = CodeController(
      text: widget.initialText,
      language: widget.language.toLowerCase() == 'sql' ? sql : go,
      readOnly: widget.readOnly,
    );
    _controller.addListener(() => widget.onChanged?.call(_controller.text));
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: BoxDecoration(
        color: const Color(0xFF12141A),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: AppColors.line),
      ),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(12),
        child: CodeTheme(
          data: CodeThemeData(styles: monokaiSublimeTheme),
          child: TextField(
            controller: _controller,
            readOnly: widget.readOnly,
            expands: widget.expands,
            minLines: widget.expands ? null : 8,
            maxLines: null,
            textAlignVertical: TextAlignVertical.top,
            cursorColor: AppColors.accent,
            style: const TextStyle(
              fontFamily: 'monospace',
              fontSize: 13.5,
              height: 1.45,
              color: AppColors.text,
            ),
            decoration: const InputDecoration(
              filled: true,
              fillColor: Color(0xFF12141A),
              contentPadding: EdgeInsets.all(12),
              border: InputBorder.none,
              enabledBorder: InputBorder.none,
              focusedBorder: InputBorder.none,
              disabledBorder: InputBorder.none,
            ),
          ),
        ),
      ),
    );
  }
}

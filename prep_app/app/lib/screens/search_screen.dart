import 'package:flutter/material.dart';

import '../api.dart';
import '../models.dart';
import '../session.dart';
import '../theme.dart';

class SearchScreen extends StatefulWidget {
  const SearchScreen({super.key, required this.api, required this.session});

  final Api api;
  final Session session;

  @override
  State<SearchScreen> createState() => _SearchScreenState();
}

class _SearchScreenState extends State<SearchScreen> {
  final _query = TextEditingController();
  SearchResult? _result;
  String? _error;
  bool _loading = false;
  bool _ai = false;

  @override
  void initState() {
    super.initState();
    widget.api
        .aiEnabled()
        .then((value) {
          if (mounted) setState(() => _ai = value);
        })
        .catchError((_) {});
  }

  @override
  void dispose() {
    _query.dispose();
    super.dispose();
  }

  Future<void> _search() async {
    final text = _query.text.trim();
    if (text.isEmpty) return;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final result = await widget.api.search(text);
      if (!mounted) return;
      setState(() {
        _result = result;
        _loading = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.message;
        _loading = false;
      });
    }
  }

  void _open(SearchHit hit) {
    final id = hit.openId;
    if (id == null) {
      showDialog<void>(
        context: context,
        builder: (context) => AlertDialog(
          backgroundColor: AppColors.surface,
          title: Text(hit.title.isEmpty ? 'Концепт' : hit.title),
          content: Text(hit.snippet),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text('Закрыть'),
            ),
          ],
        ),
      );
      return;
    }
    widget.session.openItem(id);
  }

  @override
  Widget build(BuildContext context) {
    final wide = isWide(context);
    final result = _result;
    return ListView(
      padding: EdgeInsets.fromLTRB(wide ? 28 : 16, 20, wide ? 28 : 16, 28),
      children: [
        const Text(
          'База знаний',
          style: TextStyle(
            fontSize: 28,
            fontWeight: FontWeight.w700,
            letterSpacing: -0.6,
          ),
        ),
        const SizedBox(height: 6),
        Text(
          _ai ? 'Поиск по вопросам и hard concepts, с коротким пояснением ИИ' : 'Поиск по вопросам и hard concepts. Пояснение ИИ включится, когда задан OPENROUTER_API_KEY',
          style: const TextStyle(color: AppColors.muted),
        ),
        const SizedBox(height: 16),
        TextField(
          controller: _query,
          onSubmitted: (_) => _search(),
          textInputAction: TextInputAction.search,
          decoration: InputDecoration(
            hintText: 'почему паникует закрытый канал',
            suffixIcon: IconButton(
              onPressed: _loading ? null : _search,
              icon: const Icon(Icons.search, color: AppColors.text),
            ),
          ),
        ),
        const SizedBox(height: 16),
        if (_loading)
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 32),
            child: Center(
              child: CircularProgressIndicator(color: AppColors.accent),
            ),
          )
        else if (_error != null)
          Text(_error!, style: const TextStyle(color: AppColors.bad))
        else if (result != null) ...[
          if (result.explanation.isNotEmpty) ...[
            const Text('Ответ', style: TextStyle(fontWeight: FontWeight.w600)),
            const SizedBox(height: 8),
            Panel(
              child: SelectableText(
                result.explanation,
                style: const TextStyle(height: 1.45),
              ),
            ),
            const SizedBox(height: 16),
          ],
          if (result.matches.isEmpty && result.hard.isEmpty)
            const Text(
              'В базе совпадений нет.',
              style: TextStyle(color: AppColors.muted),
            )
          else ...[
            if (result.matches.isNotEmpty)
              const Text(
                'Карточки',
                style: TextStyle(fontWeight: FontWeight.w600),
              ),
            for (final hit in result.matches)
              _HitTile(hit: hit, onTap: () => _open(hit)),
            if (result.hard.isNotEmpty) ...[
              const SizedBox(height: 8),
              const Text(
                'Hard concepts',
                style: TextStyle(fontWeight: FontWeight.w600),
              ),
              for (final hit in result.hard)
                _HitTile(hit: hit, onTap: () => _open(hit)),
            ],
          ],
        ],
      ],
    );
  }
}

class _HitTile extends StatelessWidget {
  const _HitTile({required this.hit, required this.onTap});

  final SearchHit hit;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: 8),
      child: Material(
        color: AppColors.surface,
        borderRadius: BorderRadius.circular(12),
        child: InkWell(
          borderRadius: BorderRadius.circular(12),
          onTap: onTap,
          child: Container(
            width: double.infinity,
            padding: const EdgeInsets.all(14),
            decoration: BoxDecoration(
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: AppColors.line),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Wrap(
                  spacing: 6,
                  runSpacing: 6,
                  children: [
                    if (hit.blockTitle.isNotEmpty)
                      MetaChip(text: hit.blockTitle),
                    if (hit.company.isNotEmpty)
                      MetaChip(text: hit.company, color: AppColors.warn),
                    if (hit.itemType.isNotEmpty)
                      MetaChip(text: hit.itemType, color: AppColors.muted),
                  ],
                ),
                const SizedBox(height: 8),
                Text(
                  hit.title,
                  style: const TextStyle(fontWeight: FontWeight.w600),
                ),
                if (hit.snippet.isNotEmpty && hit.snippet != hit.title) ...[
                  const SizedBox(height: 4),
                  Text(
                    hit.snippet,
                    style: const TextStyle(
                      color: AppColors.muted,
                      height: 1.35,
                    ),
                  ),
                ],
              ],
            ),
          ),
        ),
      ),
    );
  }
}

import 'package:flutter/material.dart';

import 'api.dart';
import 'screens/dashboard.dart';
import 'screens/search_screen.dart';
import 'screens/trainer.dart';
import 'session.dart';
import 'theme.dart';

void main() {
  final session = Session();
  final raw = Uri.base.queryParameters['item'];
  final id = int.tryParse(raw ?? '');
  if (id != null) {
    session.focusId = id;
    session.tab = 1;
    session.epoch = 1;
  }
  runApp(PrepApp(session: session));
}

class PrepApp extends StatefulWidget {
  const PrepApp({super.key, required this.session});

  final Session session;

  @override
  State<PrepApp> createState() => _PrepAppState();
}

class _PrepAppState extends State<PrepApp> {
  late final Api _api = Api();

  @override
  void dispose() {
    _api.client.close();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Планка 400к+',
      debugShowCheckedModeBanner: false,
      theme: buildTheme(),
      home: ListenableBuilder(
        listenable: widget.session,
        builder: (context, _) {
          final wide = isWide(context);
          final body = switch (widget.session.tab) {
            0 => DashboardScreen(api: _api, session: widget.session),
            1 => TrainerScreen(
              key: ValueKey(widget.session.epoch),
              api: _api,
              session: widget.session,
            ),
            _ => SearchScreen(api: _api, session: widget.session),
          };
          if (wide) {
            return Scaffold(
              body: SafeArea(
                child: Row(
                  children: [
                    _SideNav(session: widget.session),
                    const VerticalDivider(width: 1, color: AppColors.line),
                    Expanded(child: body),
                  ],
                ),
              ),
            );
          }
          return Scaffold(
            body: SafeArea(child: body),
            bottomNavigationBar: NavigationBar(
              backgroundColor: AppColors.surface,
              indicatorColor: AppColors.surface2,
              selectedIndex: widget.session.tab.clamp(0, 2),
              onDestinationSelected: (index) {
                if (index == 0) widget.session.showHome();
                if (index == 1) widget.session.openSprint();
                if (index == 2) widget.session.showSearch();
              },
              destinations: const [
                NavigationDestination(icon: Icon(Icons.speed), label: 'Планка'),
                NavigationDestination(icon: Icon(Icons.bolt), label: 'Спринт'),
                NavigationDestination(icon: Icon(Icons.search), label: 'Поиск'),
              ],
            ),
          );
        },
      ),
    );
  }
}

class _SideNav extends StatelessWidget {
  const _SideNav({required this.session});

  final Session session;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 212,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Padding(
              padding: EdgeInsets.symmetric(horizontal: 10, vertical: 8),
              child: Text(
                '400k+',
                style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
              ),
            ),
            const SizedBox(height: 8),
            _NavButton(
              label: 'Планка',
              selected: session.tab == 0,
              onTap: session.showHome,
            ),
            _NavButton(
              label: 'Спринт',
              selected: session.tab == 1,
              onTap: session.openSprint,
            ),
            _NavButton(
              label: 'Поиск',
              selected: session.tab == 2,
              onTap: session.showSearch,
            ),
            const Spacer(),
            const Padding(
              padding: EdgeInsets.symmetric(horizontal: 10),
              child: Text(
                'локальный тренажёр',
                style: TextStyle(color: AppColors.muted, fontSize: 12),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _NavButton extends StatelessWidget {
  const _NavButton({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 4),
      child: Material(
        color: selected ? AppColors.surface2 : Colors.transparent,
        borderRadius: BorderRadius.circular(10),
        child: InkWell(
          borderRadius: BorderRadius.circular(10),
          onTap: onTap,
          child: Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
            decoration: BoxDecoration(
              borderRadius: BorderRadius.circular(10),
              border: Border.all(
                color: selected ? AppColors.line : Colors.transparent,
              ),
            ),
            child: Text(
              label,
              style: TextStyle(
                fontWeight: selected ? FontWeight.w700 : FontWeight.w500,
              ),
            ),
          ),
        ),
      ),
    );
  }
}

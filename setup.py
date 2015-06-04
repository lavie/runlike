from setuptools import setup

setup(
    name='runlike',
    version='0.1',
    py_modules=['runlike'],
    install_requires=[
        'Click',
    ],
    entry_points='''
        [console_scripts]
        runlike=runlike:cli
    ''',
)
